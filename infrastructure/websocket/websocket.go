package websocket

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"regexp"
	"slices"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/translator"
)

const (
	maxMessageSize = 1024
	writeWait      = 6 * time.Second
	pingPeriod     = 2 * time.Second
	pongWait       = 6 * time.Second
)

var IDRegex = regexp.MustCompile("^[a-zA-Z0-9-]+$")

type failureResponse struct {
	Error            string                  `json:"error,omitempty"`
	ValidationErrors domain.ValidationErrors `json:"validationErrors,omitempty"`
}

type Websocket struct {
	requestRegistry   domain.RequestRegistry
	publishSubscriber domain.PublishSubscriber
	translator        translator.Translator

	websocketMaxMessageSize int64
	websocketWriteWait      time.Duration
	websocketPingPeriod     time.Duration
	websocketPongWait       time.Duration

	publish struct {
		ch   chan *domain.Reply
		done chan struct{}
	}

	lock               sync.RWMutex
	replyChans         []chan *domain.Reply
	subscribedSubjects map[string]struct{}
}

// Ensure Websocket implements the domain.Subscriber interface
var _ domain.Subscriber = &Websocket{}

// Ensure Websocket implements the domain.Replyer interface
var _ domain.Replyer = &Websocket{}

// make sure the websocket implements the http.Handler interface
var _ http.Handler = &Websocket{}

// make sure the websocket implements the io.Closer interface
var _ io.Closer = &Websocket{}

func NewWebsocket(
	requestRegistry domain.RequestRegistry,
	publishSubscriber domain.PublishSubscriber,
	translator translator.Translator,
) *Websocket {
	if pingPeriod >= pongWait {
		panic("pingPeriod must be less than pongWait")
	}

	ws := &Websocket{
		requestRegistry:   requestRegistry,
		publishSubscriber: publishSubscriber,
		translator:        translator,

		websocketMaxMessageSize: maxMessageSize,
		websocketWriteWait:      writeWait,
		websocketPingPeriod:     pingPeriod,
		websocketPongWait:       pongWait,

		publish: struct {
			ch   chan *domain.Reply
			done chan struct{}
		}{
			ch:   make(chan *domain.Reply),
			done: make(chan struct{}, 1),
		},
		replyChans:         make([]chan *domain.Reply, 0, 10),
		subscribedSubjects: make(map[string]struct{}),
	}

	go ws.fanoutRepliesToAllResponseChannels()

	return ws
}

func (w *Websocket) Reply(ctx context.Context, reply *domain.Reply) error {
	if w.isClosed() {
		return errors.New("connection is closed")
	}

	if len(reply.RequestID) == 0 {
		return errors.New("request id is required")
	}

	w.publish.ch <- reply

	return nil
}

func (w *Websocket) Subscribe(ctx context.Context, subject string, handler domain.MessageHandler) error {
	if err := w.publishSubscriber.Subscribe(ctx, subject, handler); err != nil {
		return err
	}

	w.lock.Lock()
	defer w.lock.Unlock()
	w.subscribedSubjects[subject] = struct{}{}

	return nil
}

func (w *Websocket) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	var upgrader = websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool {
			return true
		},
	}

	ws, err := upgrader.Upgrade(rw, r, nil)
	if err != nil {
		log.Println("upgrade:", err)

		return
	}
	defer ws.Close()

	ws.SetReadLimit(w.websocketMaxMessageSize)
	ws.SetReadDeadline(time.Now().Add(w.websocketPongWait))
	ws.SetPongHandler(func(string) error {
		ws.SetReadDeadline(time.Now().Add(w.websocketPongWait))
		return nil
	})
	ws.SetCloseHandler(func(code int, text string) error {
		log.Println("client disconnected.", "code:", code, "text:", text, "remote address:", ws.RemoteAddr().String())
		return nil
	})

	log.Println("new client connected", ws.RemoteAddr().String())
	responseChan, closeResponseChan := w.newResponseChan()
	defer closeResponseChan(w)

	go w.heartbeat(r.Context(), ws)
	go w.writeResponses(ws, responseChan)

	w.handleRequests(r.Context(), ws)
}

func (w *Websocket) Close() error {
	if !w.isClosed() {
		close(w.publish.done)
	}

	return nil
}

func (w *Websocket) isClosed() bool {
	select {
	case <-w.publish.done:
		return true
	default:
		return false
	}
}

func (w *Websocket) heartbeat(ctx context.Context, ws *websocket.Conn) {
	ticker := time.NewTicker(w.websocketPingPeriod)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			if err := ws.WriteControl(websocket.PingMessage, []byte{}, time.Now().Add(w.websocketWriteWait)); err != nil {
				log.Println("error on sending ping:", err)
			}
		case <-ctx.Done():
			return
		}
	}
}

func (w *Websocket) writeResponses(ws *websocket.Conn, responseChan <-chan *domain.Reply) {
	for reply := range responseChan {
		clientSideID, err := w.requestRegistry.GetClientSideID(reply.RequestID)
		if errors.Is(err, domain.ErrNotExists) {
			log.Println("request id not found in pendings requests")
			continue
		} else if err != nil {
			log.Println("error on getting client side request id:", err)
			w.publish.ch <- reply // retry the reply
			continue
		}

		// write back the client side request id
		serverSideID := reply.RequestID
		reply.RequestID = clientSideID

		ws.WriteJSON(reply)

		// delete the request from registry after sending the response.
		w.requestRegistry.DeleteByServerSideID(serverSideID)
	}
}

func (w *Websocket) handleRequests(ctx context.Context, ws *websocket.Conn) {
	for ctx.Err() == nil {
		var request domain.Request

		if err := ws.ReadJSON(&request); err != nil {
			log.Println("error on reading request:", err)

			break
		}

		if validationErrors, err := w.validate(&request); err != nil {
			log.Println("error on validating request:", err)
			w.writeErrorResponse(ws, request.ID, nil, err)

			continue
		} else if len(validationErrors) > 0 {
			w.writeErrorResponse(ws, request.ID, validationErrors, nil)

			continue
		}

		serverSideID, err := w.requestRegistry.Add(request.ID)
		if err != nil {
			log.Println("error on adding request to registry:", err)
			w.writeErrorResponse(ws, request.ID, nil, err)

			continue
		}

		// to be more informative, keep the client-side request id in a variable.
		clientSideID := request.ID

		// delete the request from registry if the client gets disconnected.
		defer w.requestRegistry.DeleteByServerSideID(serverSideID)

		// inject the server-side request id to the payload.
		payload, err := injectRequestId(request.Payload, serverSideID)
		if err != nil {
			log.Println("error on marshalling request:", err)
			w.writeErrorResponse(ws, clientSideID, nil, err)

			continue
		}

		if err := w.publishSubscriber.Publish(ctx, request.Subject, payload); err != nil {
			log.Println("error on publishing request:", err)
			w.writeErrorResponse(ws, clientSideID, nil, err)

			continue
		}
	}
}

func (w *Websocket) validate(request *domain.Request) (domain.ValidationErrors, error) {
	validationErrors := make(domain.ValidationErrors)

	if len(request.ID) == 0 {
		validationErrors["request_id"] = w.translator.Translate("required_field")
	}

	if len(request.ID) > 0 {
		serverSideID, err := w.requestRegistry.GetServerSideID(request.ID)
		if err != nil && !errors.Is(err, domain.ErrNotExists) {
			return validationErrors, err
		}

		if len(serverSideID) > 0 {
			validationErrors["request_id"] = w.translator.Translate("request_already_exists")
		}
	}

	if !IDRegex.MatchString(request.ID) {
		validationErrors["request_id"] = w.translator.Translate("invalid_value")
	}

	if len(request.Subject) == 0 {
		validationErrors["subject"] = w.translator.Translate("required_field")
	}

	if len(request.Subject) > 0 {
		w.lock.RLock()
		if _, ok := w.subscribedSubjects[request.Subject]; !ok {
			validationErrors["subject"] = w.translator.Translate("invalid_value")
		}
		w.lock.RUnlock()
	}

	return validationErrors, nil
}

func (w *Websocket) fanoutRepliesToAllResponseChannels() {
	for reply := range w.publish.ch {
		log.Println("publishing reply to all response channels", reply.RequestID)

		w.lock.RLock()
		for _, replyChan := range w.replyChans {
			replyChan <- reply
		}
		w.lock.RUnlock()
	}
}

func (w *Websocket) newResponseChan() (<-chan *domain.Reply, func(w *Websocket)) {
	replyChan := make(chan *domain.Reply)

	closeResponseChan := func(w *Websocket) {
		defer close(replyChan)

		w.lock.RLock()
		index := slices.Index(w.replyChans, replyChan)
		w.lock.RUnlock()

		if index == -1 {
			return
		}

		w.lock.Lock()
		defer w.lock.Unlock()
		w.replyChans = slices.Delete(w.replyChans, index, index+1)
	}

	w.lock.Lock()
	defer w.lock.Unlock()
	w.replyChans = append(w.replyChans, replyChan)

	return replyChan, closeResponseChan
}

func (w *Websocket) writeErrorResponse(
	ws *websocket.Conn,
	requestID string,
	validationErrors domain.ValidationErrors,
	err error,
) {
	log.Println("writing failure response to client", requestID, validationErrors, err)

	response := &failureResponse{
		ValidationErrors: validationErrors,
	}

	if err != nil {
		response.Error = w.translator.Translate("error_on_processing_the_request")
	}

	payload, err := json.Marshal(response)
	if err != nil {
		log.Println("error marshalling failure payload:", err)

		return
	}

	reply := &domain.Reply{
		RequestID: requestID,
		Payload:   payload,
	}

	_ = ws.WriteJSON(reply)
}

func injectRequestId(payload []byte, requestID string) ([]byte, error) {
	var request map[string]any

	if err := json.Unmarshal(payload, &request); err != nil {
		return nil, err
	}

	request["id"] = requestID

	return json.Marshal(request)
}

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
	maxMessageSize      = 1024
	writeWait           = 6 * time.Second
	pingPeriod          = 2 * time.Second
	pongWait            = 6 * time.Second
	subscriptionsPrefix = "websocket_"
)

var IDRegex = regexp.MustCompile("^[a-zA-Z0-9-]+$")

type failureResponse struct {
	Error            string                  `json:"error,omitempty"`
	ValidationErrors domain.ValidationErrors `json:"validationErrors,omitempty"`
}

type Websocket struct {
	requestRegistry   domain.RequestRegistry
	produceConsumer   domain.ProduceConsumer
	publishSubscriber domain.PublishSubscriber
	translator        translator.Translator

	websocketMaxMessageSize int64
	websocketWriteWait      time.Duration
	websocketPingPeriod     time.Duration
	websocketPongWait       time.Duration

	publish *struct {
		ch      chan *domain.Reply
		done    chan struct{}
		subject string
	}

	lock               sync.RWMutex
	replyChans         []chan *domain.Reply
	subscribedSubjects map[string]struct{}
}

// Ensure Websocket implements the domain.Consumer interface
var _ domain.Consumer = &Websocket{}

// Ensure Websocket implements the domain.Replyer interface
var _ domain.Replyer = &Websocket{}

// make sure the websocket implements the http.Handler interface
var _ http.Handler = &Websocket{}

// make sure the websocket implements the io.Closer interface
var _ io.Closer = &Websocket{}

func NewWebsocket(
	requestRegistry domain.RequestRegistry,
	produceConsumer domain.ProduceConsumer,
	publishSubscriber domain.PublishSubscriber,
	translator translator.Translator,
	repliesSubject string,
) (*Websocket, error) {
	if pingPeriod >= pongWait {
		panic("pingPeriod must be less than pongWait")
	}

	ws := &Websocket{
		requestRegistry:   requestRegistry,
		produceConsumer:   produceConsumer,
		publishSubscriber: publishSubscriber,
		translator:        translator,

		websocketMaxMessageSize: maxMessageSize,
		websocketWriteWait:      writeWait,
		websocketPingPeriod:     pingPeriod,
		websocketPongWait:       pongWait,

		publish: &struct {
			ch      chan *domain.Reply
			done    chan struct{}
			subject string
		}{
			ch:      make(chan *domain.Reply),
			done:    make(chan struct{}, 1),
			subject: subscriptionsPrefix + repliesSubject,
		},
		replyChans:         make([]chan *domain.Reply, 0, 10),
		subscribedSubjects: make(map[string]struct{}),
	}

	if err := ws.subscribeToReplies(); err != nil {
		return nil, err
	}

	go ws.fanoutRepliesToAllResponseChannels()

	return ws, nil
}

func (w *Websocket) Reply(ctx context.Context, reply *domain.Reply) error {
	select {
	case <-w.publish.done:
		return errors.New("connection is closed")
	default:
		if len(reply.RequestID) == 0 {
			return errors.New("request id is required")
		}

		// publish message, so all replicas of the application can handle the reply
		// and send it back to the client if client is connected to any of them.
		replyPayload, err := json.Marshal(reply)
		if err != nil {
			return err
		}

		return w.publishSubscriber.Publish(ctx, w.publish.subject, replyPayload)
	}
}

// Consume subscribes to the given subject and handles incoming messages using the provided handler once for each message,
// even if there are multiple replicas of the application running.
func (w *Websocket) Consume(ctx context.Context, subject string, handler domain.MessageHandler) error {
	if err := w.produceConsumer.Consume(ctx, subscriptionsPrefix+subject, handler); err != nil {
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
	select {
	case <-w.publish.done:
	default:
		close(w.publish.done)
	}

	return nil
}

func (w *Websocket) subscribeToReplies() error {
	w.lock.RLock()
	defer w.lock.RUnlock()

	return w.publishSubscriber.Subscribe(
		context.Background(),
		w.publish.subject,
		domain.MessageHandlerFunc(func(payload []byte) error {
			var reply domain.Reply

			if err := json.Unmarshal(payload, &reply); err != nil {
				log.Println("error on unmarshalling reply:", err)

				return nil
			}

			log.Println(reply)

			select {
			case w.publish.ch <- &reply:
			case <-w.publish.done:
			}

			return nil
		}),
	)
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
			select {
			case w.publish.ch <- reply: // retry the reply
			case <-w.publish.done:
				return
			}
			continue
		}

		// write back the client side request id
		serverSideID := reply.RequestID
		reply.RequestID = clientSideID

		_ = ws.SetWriteDeadline(time.Now().Add(w.websocketWriteWait))
		_ = ws.WriteJSON(reply)

		// delete the request from registry after sending the response.
		w.requestRegistry.DeleteByServerSideID(serverSideID)
	}
}

func (w *Websocket) handleRequests(ctx context.Context, ws *websocket.Conn) {
	// track in-flight server-side IDs and sweep them on disconnect
	var pendingServerSideIDs []string
	defer func() {
		for _, id := range pendingServerSideIDs {
			w.requestRegistry.DeleteByServerSideID(id)
		}
	}()

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

		pendingServerSideIDs = append(pendingServerSideIDs, serverSideID)

		// inject the server-side request id to the payload.
		payload, err := injectRequestId(request.Payload, serverSideID)
		if err != nil {
			log.Println("error on marshalling request:", err)
			w.writeErrorResponse(ws, clientSideID, nil, err)

			continue
		}

		// produce the request, so the message will be handled only once by the consumer a single replica of the application.
		if err := w.produceConsumer.Produce(ctx, subscriptionsPrefix+request.Subject, payload); err != nil {
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
	for {
		select {
		case <-w.publish.done:
			return
		case reply, ok := <-w.publish.ch:
			if !ok {
				return
			}

			log.Println("publishing reply to all response channels", reply.RequestID)

			w.lock.RLock()
			for _, replyChan := range w.replyChans {
				select {
				case replyChan <- reply:
				default:
					log.Println("response channel is full due to slow connection, skipping the reply for request id:", reply.RequestID)
				}
			}
			w.lock.RUnlock()
		}
	}
}

func (w *Websocket) newResponseChan() (<-chan *domain.Reply, func(w *Websocket)) {
	// buffered channel is used to absorb short bursts (up to 10 parallel comming responses) without blocking.
	replyChan := make(chan *domain.Reply, 10)

	closeResponseChan := func(w *Websocket) {
		defer close(replyChan)

		w.lock.Lock()
		defer w.lock.Unlock()

		index := slices.Index(w.replyChans, replyChan)
		if index == -1 {
			return
		}
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

	_ = ws.SetWriteDeadline(time.Now().Add(w.websocketWriteWait))
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

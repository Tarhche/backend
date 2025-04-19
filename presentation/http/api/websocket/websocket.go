package websocket

import (
	"context"
	"encoding/json"
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

var IDRegex = regexp.MustCompile("^[a-zA-Z0-9-]+$")

type wsHandler struct {
	// Time allowed to write a message to the peer.
	writeWait time.Duration

	// Maximum message size allowed from peer.
	maxMessageSize int64

	// Time allowed to read the next pong message from the peer.
	pongWait time.Duration

	// Send pings to peer with this period. Must be less than pongWait.
	pingPeriod time.Duration

	// Time to wait before force close on connection.
	closeGracePeriod time.Duration

	// asyncReplyChan is a channel that receives replies.
	asyncReplyChan <-chan *domain.Reply

	// responseChans is a list of channels that receive replies.
	responseChans []chan *domain.Reply

	lock sync.RWMutex

	asyncRequester domain.Requester

	translator translator.Translator
}

type failureResponse struct {
	Error            string                  `json:"error,omitempty"`
	ValidationErrors domain.ValidationErrors `json:"validationErrors,omitempty"`
}

var _ http.Handler = &wsHandler{}

func NewWsHandler(
	writeWait time.Duration,
	maxMessageSize int64,
	pongWait time.Duration,
	pingPeriod time.Duration,
	closeGracePeriod time.Duration,
	asyncReplyChan <-chan *domain.Reply,
	asyncRequester domain.Requester,
	translator translator.Translator,
) *wsHandler {
	if pingPeriod >= pongWait {
		panic("pingPeriod must be less than pongWait")
	}

	h := &wsHandler{
		writeWait:        writeWait,
		maxMessageSize:   maxMessageSize,
		pongWait:         pongWait,
		pingPeriod:       pingPeriod,
		closeGracePeriod: closeGracePeriod,
		asyncReplyChan:   asyncReplyChan,
		responseChans:    make([]chan *domain.Reply, 0, 10),
		asyncRequester:   asyncRequester,
		translator:       translator,
	}

	// fanout replies to all response channels
	go func() {
		for reply := range asyncReplyChan {
			h.lock.RLock()
			for _, responseChan := range h.responseChans {
				responseChan <- reply
			}
			h.lock.RUnlock()
		}
	}()

	return h
}

func (h *wsHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var upgrader = websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool {
			return true // TODO: check origin
		},
	}

	ws, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println("upgrade:", err)

		return
	}
	defer ws.Close()

	ws.SetReadLimit(h.maxMessageSize)
	ws.SetReadDeadline(time.Now().Add(h.pongWait))
	ws.SetPongHandler(func(string) error { ws.SetReadDeadline(time.Now().Add(h.pongWait)); return nil })

	go h.heartbeat(ws, r.Context().Done())

	log.Println("new client connected")
	responseChan := make(chan *domain.Reply)
	h.lock.Lock()
	h.responseChans = append(h.responseChans, responseChan)
	h.lock.Unlock()
	defer func() {
		h.lock.RLock()
		index := slices.Index(h.responseChans, responseChan)
		h.lock.RUnlock()

		if index != -1 {
			h.lock.Lock()
			h.responseChans = slices.Delete(h.responseChans, index, index+1)
			h.lock.Unlock()
		}

		close(responseChan)
	}()

	// TODO: we need to add a prefix to the request ID to avoid collisions/hijacking between clients
	var lock sync.RWMutex
	pendingRequestIDs := make(map[string]bool, 0)
	defer clear(pendingRequestIDs)

	// write responses to client
	go func() {
		for reply := range responseChan {
			lock.RLock()
			isBinaryMessage, ok := pendingRequestIDs[reply.RequestID]
			lock.RUnlock()

			log.Printf("pendingRequestIDs: %+v", pendingRequestIDs)
			log.Printf("isBinaryMessage: %t, ok: %t", isBinaryMessage, ok)
			log.Printf("reply: %+v", reply)

			if !ok {
				continue
			}

			replyJSON, err := json.Marshal(reply)
			if err != nil {
				log.Println("error marshalling reply:", err)
				continue
			}

			if isBinaryMessage {
				ws.WriteMessage(websocket.BinaryMessage, replyJSON)
				continue
			}

			ws.WriteMessage(websocket.TextMessage, replyJSON)
		}
	}()

	// read requests from client
	for {
		messageType, message, err := ws.ReadMessage()
		if err != nil {
			log.Println("read:", err)
			break
		}

		go h.handleMessage(ws, messageType, message, &lock, pendingRequestIDs)
	}
}

func (h *wsHandler) handleMessage(ws *websocket.Conn, messageType int, message []byte, lock *sync.RWMutex, pendingRequestIDs map[string]bool) {
	var (
		requestID        string
		validationErrors domain.ValidationErrors
		err              error
	)

	if messageType == websocket.TextMessage {
		requestID, validationErrors, err = h.handleTextMessage(ws, message)
	} else if messageType == websocket.BinaryMessage {
		requestID, validationErrors, err = h.handleBinaryMessage(ws, message)
	} else {
		log.Println("unknown message type:", messageType)
		return
	}

	if validationErrors == nil {
		validationErrors = make(domain.ValidationErrors)
	}
	defer clear(validationErrors)

	lock.RLock()
	if _, ok := pendingRequestIDs[requestID]; ok {
		validationErrors["request_id"] = h.translator.Translate("request_already_exists")
	}
	lock.RUnlock()

	// TODO: improve the code (duplicate, error handling, better structure, better validation)
	// TODO: we should not keep states on the server side, we should just send the request and get the response (replication can cause issues)
	// TODO: to fix replication issue, set the replicas to 1, after fixing the issue set it back to 2
	if len(validationErrors) > 0 || err != nil {
		failureResponse := &failureResponse{
			ValidationErrors: validationErrors,
		}

		if err != nil {
			failureResponse.Error = h.translator.Translate("error_on_processing_the_request")

			if err == domain.ErrReplierNotFound {
				failureResponse.ValidationErrors["subject"] = h.translator.Translate("invalid_value")
			}
		}

		if len(failureResponse.ValidationErrors) > 0 {
			failureResponse.ValidationErrors = validationErrors
		}

		failureResponseJSON, err := json.Marshal(failureResponse)
		if err != nil {
			log.Println("error marshalling reply payload:", err)
			return
		}

		reply := &domain.Reply{
			RequestID: requestID,
			Payload:   failureResponseJSON,
		}

		replyJSON, err := json.Marshal(reply)
		if err != nil {
			log.Println("error marshalling reply:", err)
			return
		}

		if messageType == websocket.TextMessage {
			ws.WriteMessage(websocket.TextMessage, replyJSON)
		}

		if messageType == websocket.BinaryMessage {
			ws.WriteMessage(websocket.BinaryMessage, replyJSON)
		}

		return
	}

	lock.Lock()
	pendingRequestIDs[requestID] = false
	if messageType == websocket.BinaryMessage {
		pendingRequestIDs[requestID] = true
	}
	lock.Unlock()
}

func (h *wsHandler) handleTextMessage(ws *websocket.Conn, message []byte) (string, domain.ValidationErrors, error) {
	log.Println("received text message:", string(message))

	var request domain.Request
	if err := json.Unmarshal(message, &request); err != nil {
		return "", nil, err
	}

	if validationErrors := h.validateRequest(request); len(validationErrors) > 0 {
		return request.ID, validationErrors, nil
	}

	if err := h.asyncRequester.Request(context.Background(), &request); err != nil {
		return request.ID, nil, err
	}

	return request.ID, nil, nil
}

func (h *wsHandler) handleBinaryMessage(ws *websocket.Conn, message []byte) (string, domain.ValidationErrors, error) {
	log.Println("received binary message:", string(message))

	var request domain.Request
	if err := json.Unmarshal(message, &request); err != nil {
		return "", nil, err
	}

	if validationErrors := h.validateRequest(request); len(validationErrors) > 0 {
		return request.ID, validationErrors, nil
	}

	if err := h.asyncRequester.Request(context.Background(), &request); err != nil {
		return request.ID, nil, err
	}

	return request.ID, nil, nil
}

func (h *wsHandler) validateRequest(request domain.Request) domain.ValidationErrors {
	validationErrors := make(domain.ValidationErrors)

	if len(request.ID) == 0 {
		validationErrors["request_id"] = h.translator.Translate("required_field")
	}

	if !IDRegex.MatchString(request.ID) {
		validationErrors["request_id"] = h.translator.Translate("invalid_value")
	}

	if len(request.Subject) == 0 {
		validationErrors["subject"] = h.translator.Translate("required_field")
	}

	return validationErrors
}

func (h *wsHandler) heartbeat(ws *websocket.Conn, done <-chan struct{}) {
	ticker := time.NewTicker(h.pingPeriod)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			if err := ws.WriteControl(websocket.PingMessage, []byte{}, time.Now().Add(h.writeWait)); err != nil {
				log.Println("ping:", err)
			}
		case <-done:
			return
		}
	}
}

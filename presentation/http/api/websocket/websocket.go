package websocket

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"slices"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/khanzadimahdi/testproject/domain"
)

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
	var upgrader = websocket.Upgrader{}

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

	var lock sync.RWMutex
	pendingRequestIDs := make(map[string]bool, 0)

	// write responses to client
	go func() {
		for reply := range responseChan {
			lock.RLock()
			binaryMessage, ok := pendingRequestIDs[reply.RequestID]
			lock.RUnlock()

			if !ok {
				continue
			}

			json, err := json.Marshal(reply)
			if err != nil {
				log.Println("error marshalling reply:", err)
				continue
			}

			if binaryMessage {
				ws.WriteMessage(websocket.BinaryMessage, json)
				continue
			}

			ws.WriteMessage(websocket.TextMessage, json)
		}
	}()

	// read requests from client
	for {
		messageType, message, err := ws.ReadMessage()
		if err == io.EOF {
			break
		} else if err != nil {
			log.Println("read:", err)
			continue
		}

		// TODO: improve the code (duplicate, error handling, better structure, better validation)
		if messageType == websocket.TextMessage {
			requestID, err := h.handleTextMessage(ws, message)
			if err == domain.ErrReplierNotFound {
				log.Println("unknown subject")
				continue
			}

			if err != nil {
				log.Println("error handling text message:", err)
				continue
			}

			lock.Lock()
			pendingRequestIDs[requestID] = false
			lock.Unlock()

			continue
		}

		if messageType == websocket.BinaryMessage {
			requestID, err := h.handleBinaryMessage(ws, message)
			if err == domain.ErrReplierNotFound {
				log.Println("unknown subject")
				continue
			}

			if err != nil {
				log.Println("error handling binary message:", err)
				continue
			}

			lock.Lock()
			pendingRequestIDs[requestID] = true
			lock.Unlock()

			continue
		}
	}
}

func (h *wsHandler) handleTextMessage(ws *websocket.Conn, message []byte) (string, error) {
	log.Println("received text message:", string(message))

	var request domain.Request
	if err := json.Unmarshal(message, &request); err != nil {
		log.Println("error unmarshalling request:", err)
		return "", err
	}

	if err := h.validateRequest(request); err != nil {
		log.Println("invalid request:", err)
		return "", err
	}

	err := h.handleRequest(request)

	return request.ID, err
}

func (h *wsHandler) handleBinaryMessage(ws *websocket.Conn, message []byte) (string, error) {
	log.Println("received binary message:", string(message))

	var request domain.Request
	if err := json.Unmarshal(message, &request); err != nil {
		log.Println("error unmarshalling request:", err)
		return "", err
	}

	if err := h.validateRequest(request); err != nil {
		log.Println("invalid request:", err)
		return "", err
	}

	err := h.handleRequest(request)

	return request.ID, err
}

func (h *wsHandler) handleRequest(request domain.Request) error {
	err := h.asyncRequester.Request(context.Background(), &request)
	if err == domain.ErrReplierNotFound {
		return errors.New("unknown subject")
	}

	return err
}

func (h *wsHandler) validateRequest(request domain.Request) error {
	if len(request.ID) == 0 {
		return errors.New("request ID is required")
	}

	if len(request.Subject) == 0 {
		return errors.New("subject is required")
	}

	return nil
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

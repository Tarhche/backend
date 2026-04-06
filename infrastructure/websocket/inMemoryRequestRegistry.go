package websocket

import (
	"sync"

	"github.com/gofrs/uuid/v5"
	"github.com/khanzadimahdi/testproject/domain"
)

type InMemoryRequestRegistry struct {
	lock           sync.RWMutex
	clientToServer map[string]string
	serverToClient map[string]string
}

// make sure the InMemoryRequestRegistry implements the domain.RequestRegistry interface
var _ domain.RequestRegistry = &InMemoryRequestRegistry{}

// NewInMemoryRequestRegistry initializes a new InMemoryRequestRegistry
func NewInMemoryRequestRegistry(initialSize int) *InMemoryRequestRegistry {
	return &InMemoryRequestRegistry{
		clientToServer: make(map[string]string, initialSize),
		serverToClient: make(map[string]string, initialSize),
	}
}

// Add registers a new client and generates a serverSideID
func (r *InMemoryRequestRegistry) Add(clientSideID string) (string, error) {
	r.lock.Lock()
	defer r.lock.Unlock()

	if sid, exists := r.clientToServer[clientSideID]; exists {
		return sid, domain.ErrAlreadyExists
	}

	serverSideID, err := generateServerID()
	if err != nil {
		return "", err
	}

	r.clientToServer[clientSideID] = serverSideID
	r.serverToClient[serverSideID] = clientSideID

	return serverSideID, nil
}

// GetClientSideID returns the clientSideID for a given serverSideID
func (r *InMemoryRequestRegistry) GetClientSideID(serverSideID string) (string, error) {
	r.lock.RLock()
	defer r.lock.RUnlock()

	clientSideID, exists := r.serverToClient[serverSideID]
	if !exists {
		return "", domain.ErrNotExists
	}

	return clientSideID, nil
}

// GetServerSideID returns the serverSideID for a given clientSideID
func (r *InMemoryRequestRegistry) GetServerSideID(clientSideID string) (string, error) {
	r.lock.RLock()
	defer r.lock.RUnlock()

	serverSideID, exists := r.clientToServer[clientSideID]
	if !exists {
		return "", domain.ErrNotExists
	}

	return serverSideID, nil
}

// DeleteByServerSideID removes the mapping by serverSideID
func (r *InMemoryRequestRegistry) DeleteByServerSideID(serverSideID string) error {
	r.lock.Lock()
	defer r.lock.Unlock()

	clientSideID, exists := r.serverToClient[serverSideID]
	if !exists {
		return domain.ErrNotExists
	}

	delete(r.serverToClient, serverSideID)
	delete(r.clientToServer, clientSideID)

	return nil
}

// generateServerID creates a unique random server ID
func generateServerID() (string, error) {
	uuid, err := uuid.NewV7()
	if err != nil {
		return "", err
	}

	return uuid.String(), nil
}

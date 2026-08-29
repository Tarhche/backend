package routing

import (
	"sync"

	"github.com/gofrs/uuid/v5"
	"github.com/khanzadimahdi/testproject/domain"
)

// RequestRegistry maps the ids a client chooses to the ids the rest of the
// system routes on, for the length of one connection.
type RequestRegistry interface {
	Add(clientSideID string) (string, error)
	GetClientSideID(serverSideID string) (string, error)
	GetServerSideID(clientSideID string) (string, error)
	DeleteByServerSideID(serverSideID string) error
}

type InMemoryRegistry struct {
	lock           sync.RWMutex
	clientToServer map[string]string
	serverToClient map[string]string
}

// make sure the InMemoryRegistry implements the RequestRegistry interface
var _ RequestRegistry = &InMemoryRegistry{}

// NewInMemoryRegistry initializes a new InMemoryRegistry
func NewInMemoryRegistry(initialSize int) *InMemoryRegistry {
	return &InMemoryRegistry{
		clientToServer: make(map[string]string, initialSize),
		serverToClient: make(map[string]string, initialSize),
	}
}

// Add registers a new client and generates a serverSideID
func (r *InMemoryRegistry) Add(clientSideID string) (string, error) {
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
func (r *InMemoryRegistry) GetClientSideID(serverSideID string) (string, error) {
	r.lock.RLock()
	defer r.lock.RUnlock()

	clientSideID, exists := r.serverToClient[serverSideID]
	if !exists {
		return "", domain.ErrNotExists
	}

	return clientSideID, nil
}

// GetServerSideID returns the serverSideID for a given clientSideID
func (r *InMemoryRegistry) GetServerSideID(clientSideID string) (string, error) {
	r.lock.RLock()
	defer r.lock.RUnlock()

	serverSideID, exists := r.clientToServer[clientSideID]
	if !exists {
		return "", domain.ErrNotExists
	}

	return serverSideID, nil
}

// DeleteByServerSideID removes the mapping by serverSideID
func (r *InMemoryRegistry) DeleteByServerSideID(serverSideID string) error {
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

package models

import (
	"sync"
)

// ConcurrentStateStore provides thread-safe storage for workflow states
type ConcurrentStateStore struct {
	States    map[string]*CoordinatorState // Map messageID to state
	Mutex     sync.RWMutex
	onCleanup func(messageID string) // Callback for cleanup when state is removed
}

// NewConcurrentStateStore creates a new thread-safe state store
func NewConcurrentStateStore() StateStore {
	return &ConcurrentStateStore{
		States: make(map[string]*CoordinatorState),
	}
}

// SetCleanupHandler sets a callback function to be called when a state is removed
// This allows other components to clean up their caches when a workflow completes
func (s *ConcurrentStateStore) SetCleanupHandler(handler func(messageID string)) {
	s.onCleanup = handler
}

// GetState returns the state for a messageID, creating it if needed
func (s *ConcurrentStateStore) GetState(messageID string) *CoordinatorState {
	s.Mutex.RLock()
	state, exists := s.States[messageID]
	s.Mutex.RUnlock()

	if !exists {
		s.Mutex.Lock()
		// Double-check after acquiring write lock
		state, exists = s.States[messageID]
		if !exists {
			state = NewCoordinatorState()
			s.States[messageID] = state
		}
		s.Mutex.Unlock()
	}

	return state
}

// RemoveState removes a state when workflow is complete
// and triggers cleanup of related caches via the cleanup handler
func (s *ConcurrentStateStore) RemoveState(messageID string) {
	s.Mutex.Lock()
	delete(s.States, messageID)
	s.Mutex.Unlock()

	// Trigger cleanup callback if set
	if s.onCleanup != nil {
		s.onCleanup(messageID)
	}
}

// GetActiveWorkflows returns a list of active workflow message IDs
func (s *ConcurrentStateStore) GetActiveWorkflows() []string {
	s.Mutex.RLock()
	defer s.Mutex.RUnlock()

	ids := make([]string, 0, len(s.States))
	for id := range s.States {
		ids = append(ids, id)
	}

	return ids
}

// SetState sets a specific state for a messageID (used for checkpoint restoration)
func (s *ConcurrentStateStore) SetState(messageID string, state *CoordinatorState) {
	s.Mutex.Lock()
	defer s.Mutex.Unlock()
	s.States[messageID] = state
}

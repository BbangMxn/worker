package session

import (
	"fmt"
	"sync"
	"time"

	"worker_server/core/agent/tools"

	"github.com/google/uuid"
)

// =============================================================================
// Session Management
// =============================================================================

// Session represents a conversation session
type Session struct {
	ID        string    `json:"id"`
	UserID    uuid.UUID `json:"user_id"`
	Messages  []Message `json:"messages"`
	CreatedAt time.Time `json:"created_at"`
	LastUsed  time.Time `json:"last_used"`
}

// Message represents a single message in a session
type Message struct {
	Role    string    `json:"role"`
	Content string    `json:"content"`
	Time    time.Time `json:"time"`
}

// AddMessage adds a message to the session
func (s *Session) AddMessage(role, content string) {
	s.Messages = append(s.Messages, Message{
		Role:    role,
		Content: content,
		Time:    time.Now(),
	})
	s.LastUsed = time.Now()

	// Keep only last 20 messages
	if len(s.Messages) > 20 {
		s.Messages = s.Messages[len(s.Messages)-20:]
	}
}

// GetRecentContext returns recent conversation for context
func (s *Session) GetRecentContext(limit int) string {
	if limit > len(s.Messages) {
		limit = len(s.Messages)
	}

	context := ""
	for _, msg := range s.Messages[len(s.Messages)-limit:] {
		context += fmt.Sprintf("%s: %s\n", msg.Role, msg.Content)
	}
	return context
}

// =============================================================================
// Session Manager
// =============================================================================

// Manager manages conversation sessions with TTL support
type Manager struct {
	mu       sync.RWMutex
	sessions map[string]*Session
	ttl      time.Duration
	stopCh   chan struct{}
}

// NewManager creates a new session manager with default 30 minute TTL
func NewManager() *Manager {
	return NewManagerWithTTL(30 * time.Minute)
}

// NewManagerWithTTL creates a new session manager with custom TTL
func NewManagerWithTTL(ttl time.Duration) *Manager {
	m := &Manager{
		sessions: make(map[string]*Session),
		ttl:      ttl,
		stopCh:   make(chan struct{}),
	}
	go m.cleanupLoop()
	return m
}

// GetOrCreate gets an existing session or creates a new one
func (m *Manager) GetOrCreate(sessionID string, userID uuid.UUID) *Session {
	m.mu.Lock()
	defer m.mu.Unlock()

	if sessionID == "" {
		sessionID = uuid.New().String()
	}

	if session, ok := m.sessions[sessionID]; ok {
		session.LastUsed = time.Now()
		return session
	}

	session := &Session{
		ID:        sessionID,
		UserID:    userID,
		Messages:  []Message{},
		CreatedAt: time.Now(),
		LastUsed:  time.Now(),
	}
	m.sessions[sessionID] = session
	return session
}

// Get retrieves a session by ID
func (m *Manager) Get(sessionID string) *Session {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if session, ok := m.sessions[sessionID]; ok {
		return session
	}
	return nil
}

// Delete removes a session
func (m *Manager) Delete(sessionID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.sessions, sessionID)
}

// Count returns the number of active sessions
func (m *Manager) Count() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.sessions)
}

// cleanupLoop periodically removes expired sessions
func (m *Manager) cleanupLoop() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			m.cleanup()
		case <-m.stopCh:
			return
		}
	}
}

// cleanup removes expired sessions
func (m *Manager) cleanup() {
	m.mu.Lock()
	defer m.mu.Unlock()

	now := time.Now()
	for id, session := range m.sessions {
		if now.Sub(session.LastUsed) > m.ttl {
			delete(m.sessions, id)
		}
	}
}

// Stop stops the cleanup goroutine
func (m *Manager) Stop() {
	close(m.stopCh)
}

// =============================================================================
// Proposal Store
// =============================================================================

// ProposalStore manages pending proposals with TTL support
type ProposalStore struct {
	mu        sync.RWMutex
	proposals map[string]map[string]*tools.ActionProposal // userID -> proposalID -> proposal
	stopCh    chan struct{}
}

// NewProposalStore creates a new proposal store with cleanup
func NewProposalStore() *ProposalStore {
	s := &ProposalStore{
		proposals: make(map[string]map[string]*tools.ActionProposal),
		stopCh:    make(chan struct{}),
	}
	go s.cleanupLoop()
	return s
}

// Store saves a proposal for a user
func (s *ProposalStore) Store(userID uuid.UUID, proposal *tools.ActionProposal) {
	s.mu.Lock()
	defer s.mu.Unlock()

	uid := userID.String()
	if s.proposals[uid] == nil {
		s.proposals[uid] = make(map[string]*tools.ActionProposal)
	}
	s.proposals[uid][proposal.ID] = proposal
}

// Get retrieves a non-expired proposal
func (s *ProposalStore) Get(userID uuid.UUID, proposalID string) *tools.ActionProposal {
	s.mu.RLock()
	defer s.mu.RUnlock()

	uid := userID.String()
	if s.proposals[uid] == nil {
		return nil
	}

	proposal := s.proposals[uid][proposalID]
	if proposal == nil || time.Now().After(proposal.ExpiresAt) {
		return nil
	}
	return proposal
}

// GetAll returns all non-expired proposals for a user (alias for List)
func (s *ProposalStore) GetAll(userID uuid.UUID) []*tools.ActionProposal {
	return s.List(userID)
}

// List returns all non-expired proposals for a user
func (s *ProposalStore) List(userID uuid.UUID) []*tools.ActionProposal {
	s.mu.RLock()
	defer s.mu.RUnlock()

	uid := userID.String()
	if s.proposals[uid] == nil {
		return nil
	}

	now := time.Now()
	result := make([]*tools.ActionProposal, 0)
	for _, proposal := range s.proposals[uid] {
		if now.Before(proposal.ExpiresAt) {
			result = append(result, proposal)
		}
	}
	return result
}

// Remove deletes a proposal
func (s *ProposalStore) Remove(userID uuid.UUID, proposalID string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	uid := userID.String()
	if s.proposals[uid] != nil {
		delete(s.proposals[uid], proposalID)
	}
}

// Count returns the total number of proposals
func (s *ProposalStore) Count() int {
	s.mu.RLock()
	defer s.mu.RUnlock()

	count := 0
	for _, userProposals := range s.proposals {
		count += len(userProposals)
	}
	return count
}

// cleanupLoop periodically removes expired proposals
func (s *ProposalStore) cleanupLoop() {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			s.cleanup()
		case <-s.stopCh:
			return
		}
	}
}

// cleanup removes all expired proposals
func (s *ProposalStore) cleanup() {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	for uid, userProposals := range s.proposals {
		for pid, proposal := range userProposals {
			if now.After(proposal.ExpiresAt) {
				delete(userProposals, pid)
			}
		}
		// Remove empty user maps
		if len(userProposals) == 0 {
			delete(s.proposals, uid)
		}
	}
}

// Stop stops the cleanup goroutine
func (s *ProposalStore) Stop() {
	close(s.stopCh)
}

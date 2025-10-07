package review

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"pudl/internal/errors"
)

// SessionState represents the current state of a review session
type SessionState string

const (
	SessionInProgress SessionState = "IN_PROGRESS"
	SessionCompleted  SessionState = "COMPLETED"
	SessionAborted    SessionState = "ABORTED"
)

// ReviewStatus represents the status of an individual review item
type ReviewStatus string

const (
	StatusPending    ReviewStatus = "PENDING"
	StatusAccepted   ReviewStatus = "ACCEPTED"
	StatusReassigned ReviewStatus = "REASSIGNED"
	StatusCreated    ReviewStatus = "CREATED"
	StatusSkipped    ReviewStatus = "SKIPPED"
)

// ReviewItem represents a single data item to be reviewed
type ReviewItem struct {
	EntryID         string                 `json:"entry_id"`
	Data            interface{}            `json:"data"`
	CurrentSchema   string                 `json:"current_schema"`
	SuggestedSchema string                 `json:"suggested_schema,omitempty"`
	Status          ReviewStatus           `json:"status"`
	NewSchema       string                 `json:"new_schema,omitempty"`
	ValidationError string                 `json:"validation_error,omitempty"`
	Metadata        map[string]interface{} `json:"metadata,omitempty"`
}

// SchemaChange represents a change made during the review session
type SchemaChange struct {
	EntryID     string    `json:"entry_id"`
	OldSchema   string    `json:"old_schema"`
	NewSchema   string    `json:"new_schema"`
	ChangeType  string    `json:"change_type"` // "reassign", "create", "accept"
	Timestamp   time.Time `json:"timestamp"`
	SchemaPath  string    `json:"schema_path,omitempty"` // For newly created schemas
}

// ReviewSession manages the state of a schema review session
type ReviewSession struct {
	SessionID    string         `json:"session_id"`
	Items        []ReviewItem   `json:"items"`
	CurrentIndex int            `json:"current_index"`
	State        SessionState   `json:"state"`
	Changes      []SchemaChange `json:"changes"`
	StartTime    time.Time      `json:"start_time"`
	EndTime      *time.Time     `json:"end_time,omitempty"`
	Filter       SessionFilter  `json:"filter"`
}

// SessionFilter defines criteria for selecting items to review
type SessionFilter struct {
	Schema     string `json:"schema,omitempty"`
	Origin     string `json:"origin,omitempty"`
	Format     string `json:"format,omitempty"`
	MaxItems   int    `json:"max_items,omitempty"`
	OnlyUnknown bool  `json:"only_unknown,omitempty"`
}

// SessionManager handles review session persistence and lifecycle
type SessionManager struct {
	sessionPath string
}

// NewSessionManager creates a new session manager
func NewSessionManager(pudlHome string) *SessionManager {
	sessionPath := filepath.Join(pudlHome, "review")
	os.MkdirAll(sessionPath, 0755)
	return &SessionManager{
		sessionPath: sessionPath,
	}
}

// CreateSession creates a new review session with the given items
func (sm *SessionManager) CreateSession(items []ReviewItem, filter SessionFilter) (*ReviewSession, error) {
	sessionID := generateSessionID()
	
	session := &ReviewSession{
		SessionID:    sessionID,
		Items:        items,
		CurrentIndex: 0,
		State:        SessionInProgress,
		Changes:      []SchemaChange{},
		StartTime:    time.Now(),
		Filter:       filter,
	}

	if err := sm.SaveSession(session); err != nil {
		return nil, errors.WrapError(
			errors.ErrCodeFileSystem,
			"Failed to save review session",
			err,
		)
	}

	return session, nil
}

// LoadSession loads an existing session by ID
func (sm *SessionManager) LoadSession(sessionID string) (*ReviewSession, error) {
	sessionFile := filepath.Join(sm.sessionPath, sessionID+".json")
	
	data, err := os.ReadFile(sessionFile)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, errors.NewFileNotFoundError(sessionFile)
		}
		return nil, errors.WrapError(
			errors.ErrCodeFileSystem,
			"Failed to load review session",
			err,
		)
	}

	var session ReviewSession
	if err := json.Unmarshal(data, &session); err != nil {
		return nil, errors.NewParsingError("JSON", err)
	}

	return &session, nil
}

// SaveSession saves the session state to disk
func (sm *SessionManager) SaveSession(session *ReviewSession) error {
	sessionFile := filepath.Join(sm.sessionPath, session.SessionID+".json")
	
	data, err := json.MarshalIndent(session, "", "  ")
	if err != nil {
		return errors.NewSystemError("Failed to serialize review session", err)
	}

	if err := os.WriteFile(sessionFile, data, 0644); err != nil {
		return errors.WrapError(
			errors.ErrCodeFileSystem,
			"Failed to save review session",
			err,
		)
	}

	return nil
}

// ListSessions returns all available review sessions
func (sm *SessionManager) ListSessions() ([]ReviewSession, error) {
	files, err := filepath.Glob(filepath.Join(sm.sessionPath, "*.json"))
	if err != nil {
		return nil, errors.WrapError(
			errors.ErrCodeFileSystem,
			"Failed to list review sessions",
			err,
		)
	}

	var sessions []ReviewSession
	for _, file := range files {
		data, err := os.ReadFile(file)
		if err != nil {
			continue // Skip corrupted files
		}

		var session ReviewSession
		if err := json.Unmarshal(data, &session); err != nil {
			continue // Skip corrupted files
		}

		sessions = append(sessions, session)
	}

	return sessions, nil
}

// DeleteSession removes a session file
func (sm *SessionManager) DeleteSession(sessionID string) error {
	sessionFile := filepath.Join(sm.sessionPath, sessionID+".json")
	
	if err := os.Remove(sessionFile); err != nil {
		if os.IsNotExist(err) {
			return errors.NewFileNotFoundError(sessionFile)
		}
		return errors.WrapError(
			errors.ErrCodeFileSystem,
			"Failed to delete review session",
			err,
		)
	}

	return nil
}

// GetCurrentItem returns the current item being reviewed
func (s *ReviewSession) GetCurrentItem() *ReviewItem {
	if s.CurrentIndex >= len(s.Items) {
		return nil
	}
	return &s.Items[s.CurrentIndex]
}

// NextItem advances to the next item
func (s *ReviewSession) NextItem() bool {
	s.CurrentIndex++
	return s.CurrentIndex < len(s.Items)
}

// PreviousItem goes back to the previous item
func (s *ReviewSession) PreviousItem() bool {
	if s.CurrentIndex > 0 {
		s.CurrentIndex--
		return true
	}
	return false
}

// IsComplete returns true if all items have been reviewed
func (s *ReviewSession) IsComplete() bool {
	return s.CurrentIndex >= len(s.Items)
}

// AddChange records a schema change
func (s *ReviewSession) AddChange(change SchemaChange) {
	change.Timestamp = time.Now()
	s.Changes = append(s.Changes, change)
}

// GetProgress returns the current progress as a percentage
func (s *ReviewSession) GetProgress() float64 {
	if len(s.Items) == 0 {
		return 100.0
	}
	return float64(s.CurrentIndex) / float64(len(s.Items)) * 100.0
}

// GetSummary returns a summary of the session
func (s *ReviewSession) GetSummary() map[string]int {
	summary := map[string]int{
		"total":      len(s.Items),
		"pending":    0,
		"accepted":   0,
		"reassigned": 0,
		"created":    0,
		"skipped":    0,
	}

	for _, item := range s.Items {
		switch item.Status {
		case StatusPending:
			summary["pending"]++
		case StatusAccepted:
			summary["accepted"]++
		case StatusReassigned:
			summary["reassigned"]++
		case StatusCreated:
			summary["created"]++
		case StatusSkipped:
			summary["skipped"]++
		}
	}

	return summary
}

// generateSessionID creates a unique session identifier
func generateSessionID() string {
	return fmt.Sprintf("review-%d", time.Now().Unix())
}

// Package conversation provides conversation history management for multi-turn dialogues.
package conversation

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// Message represents a single message in a conversation.
type Message struct {
	ID             string      `json:"id"`
	ConversationID string      `json:"conversation_id"`
	Role           string      `json:"role"` // "user" or "assistant"
	Content        string      `json:"content"`
	ImageData      string      `json:"image_data,omitempty"`
	Sources        []SourceRef `json:"sources,omitempty"`
	CreatedAt      time.Time   `json:"created_at"`
}

// SourceRef represents a reference to a source document.
type SourceRef struct {
	DocumentName string `json:"document_name"`
	ChunkIndex   int    `json:"chunk_index,omitempty"`
}

// Manager handles conversation history operations.
type Manager struct {
	readDB  *sql.DB
	writeDB *sql.DB
}

// NewManager creates a new conversation manager.
func NewManager(readDB, writeDB *sql.DB) *Manager {
	return &Manager{
		readDB:  readDB,
		writeDB: writeDB,
	}
}

// GetOrCreateConversation retrieves an existing conversation or creates a new one.
// If conversationID is provided and valid, returns it.
// Otherwise, always creates a new conversation to ensure proper session isolation.
func (m *Manager) GetOrCreateConversation(userID, sessionID, productID string) (string, error) {
	if userID == "" {
		return "", fmt.Errorf("userID is required")
	}

	// Always create a new conversation
	// The caller should pass an existing conversation_id if they want to continue a conversation
	conversationID := uuid.New().String()
	_, err := m.writeDB.Exec(`
		INSERT INTO conversations (id, user_id, session_id, product_id, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?)
	`, conversationID, userID, sessionID, productID, time.Now(), time.Now())
	if err != nil {
		return "", fmt.Errorf("create conversation: %w", err)
	}

	return conversationID, nil
}

// ValidateConversation checks if a conversation exists and belongs to the user.
func (m *Manager) ValidateConversation(conversationID, userID string) (bool, error) {
	if conversationID == "" || userID == "" {
		return false, nil
	}

	var count int
	err := m.readDB.QueryRow(`
		SELECT COUNT(*) FROM conversations
		WHERE id = ? AND user_id = ?
	`, conversationID, userID).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("validate conversation: %w", err)
	}

	return count > 0, nil
}

// AddMessage adds a message to the conversation history.
func (m *Manager) AddMessage(conversationID, role, content, imageData string, sources []SourceRef) error {
	if conversationID == "" {
		return fmt.Errorf("conversationID is required")
	}
	if role != "user" && role != "assistant" {
		return fmt.Errorf("invalid role: %s", role)
	}

	messageID := uuid.New().String()
	sourcesJSON := ""
	if len(sources) > 0 {
		data, err := json.Marshal(sources)
		if err != nil {
			return fmt.Errorf("marshal sources: %w", err)
		}
		sourcesJSON = string(data)
	}

	_, err := m.writeDB.Exec(`
		INSERT INTO conversation_messages (id, conversation_id, role, content, image_data, sources, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`, messageID, conversationID, role, content, imageData, sourcesJSON, time.Now())
	if err != nil {
		return fmt.Errorf("insert message: %w", err)
	}

	// Update conversation's updated_at timestamp
	_, err = m.writeDB.Exec(`
		UPDATE conversations SET updated_at = ? WHERE id = ?
	`, time.Now(), conversationID)
	if err != nil {
		return fmt.Errorf("update conversation timestamp: %w", err)
	}

	return nil
}

// GetRecentMessages retrieves the most recent N messages from a conversation.
func (m *Manager) GetRecentMessages(conversationID string, limit int) ([]Message, error) {
	if conversationID == "" {
		return nil, nil
	}

	query := `
		SELECT id, conversation_id, role, content, image_data, sources, created_at
		FROM conversation_messages
		WHERE conversation_id = ?
		ORDER BY created_at DESC
		LIMIT ?
	`
	rows, err := m.readDB.Query(query, conversationID, limit)
	if err != nil {
		return nil, fmt.Errorf("query messages: %w", err)
	}
	defer rows.Close()

	var messages []Message
	for rows.Next() {
		var msg Message
		var sourcesJSON string
		err := rows.Scan(&msg.ID, &msg.ConversationID, &msg.Role, &msg.Content, &msg.ImageData, &sourcesJSON, &msg.CreatedAt)
		if err != nil {
			return nil, fmt.Errorf("scan message: %w", err)
		}

		if sourcesJSON != "" {
			if err := json.Unmarshal([]byte(sourcesJSON), &msg.Sources); err != nil {
				return nil, fmt.Errorf("unmarshal sources: %w", err)
			}
		}

		messages = append(messages, msg)
	}

	// Reverse to get chronological order (oldest first)
	for i, j := 0, len(messages)-1; i < j; i, j = i+1, j-1 {
		messages[i], messages[j] = messages[j], messages[i]
	}

	return messages, nil
}

// CleanOldConversations deletes conversations older than the specified duration.
func (m *Manager) CleanOldConversations(olderThan time.Duration) error {
	cutoff := time.Now().Add(-olderThan)
	_, err := m.writeDB.Exec(`
		DELETE FROM conversations WHERE updated_at < ?
	`, cutoff)
	if err != nil {
		return fmt.Errorf("clean old conversations: %w", err)
	}
	return nil
}


// GetMostRecentConversation retrieves the most recent conversation for a user and product.
// This is used for auto-linking when the frontend doesn't provide a conversation_id.
func (m *Manager) GetMostRecentConversation(userID, productID string, maxAge time.Duration) (string, error) {
	if userID == "" {
		return "", nil
	}

	cutoff := time.Now().Add(-maxAge)
	var conversationID string
	err := m.readDB.QueryRow(`
		SELECT id FROM conversations
		WHERE user_id = ? AND product_id = ? AND updated_at > ?
		ORDER BY updated_at DESC
		LIMIT 1
	`, userID, productID, cutoff).Scan(&conversationID)
	
	if err == sql.ErrNoRows {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("query recent conversation: %w", err)
	}

	return conversationID, nil
}

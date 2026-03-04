// Package db provides conversation table schema definitions.
package db

import (
	"database/sql"
	"fmt"
)

// createConversationTables creates the conversations and conversation_messages tables.
func createConversationTables(db *sql.DB) error {
	tables := []string{
		`CREATE TABLE IF NOT EXISTS conversations (
			id TEXT PRIMARY KEY,
			user_id TEXT NOT NULL,
			session_id TEXT DEFAULT '',
			product_id TEXT DEFAULT '',
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS conversation_messages (
			id TEXT PRIMARY KEY,
			conversation_id TEXT NOT NULL,
			role TEXT NOT NULL,
			content TEXT NOT NULL,
			image_data TEXT DEFAULT '',
			sources TEXT DEFAULT '',
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (conversation_id) REFERENCES conversations(id) ON DELETE CASCADE
		)`,
	}

	for _, ddl := range tables {
		if _, err := db.Exec(ddl); err != nil {
			return fmt.Errorf("failed to create conversation table: %w", err)
		}
	}
	return nil
}

// createConversationIndexes creates indexes for conversation tables.
func createConversationIndexes(db *sql.DB) error {
	indexes := []string{
		`CREATE INDEX IF NOT EXISTS idx_conversations_user_id ON conversations(user_id)`,
		`CREATE INDEX IF NOT EXISTS idx_conversations_session_id ON conversations(session_id)`,
		`CREATE INDEX IF NOT EXISTS idx_conversations_updated_at ON conversations(updated_at)`,
		`CREATE INDEX IF NOT EXISTS idx_conversation_messages_conversation_id ON conversation_messages(conversation_id)`,
		`CREATE INDEX IF NOT EXISTS idx_conversation_messages_created_at ON conversation_messages(created_at)`,
	}

	for _, idx := range indexes {
		if _, err := db.Exec(idx); err != nil {
			return fmt.Errorf("failed to create conversation index: %w", err)
		}
	}
	return nil
}

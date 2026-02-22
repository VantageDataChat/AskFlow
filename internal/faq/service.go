// Package faq provides FAQ (frequently asked questions) management per product.
// Each user query is recorded and similar questions are merged by increasing weight.
// The top-weighted entries surface as FAQ for the chat UI.
// Admins can manually add, reorder, edit, and delete FAQ entries.
package faq

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"fmt"
	"strings"
	"time"
)

// Entry represents a single FAQ entry scoped to a product.
type Entry struct {
	ID        string    `json:"id"`
	ProductID string    `json:"product_id"`
	Question  string    `json:"question"`
	Weight    int       `json:"weight"`
	SortOrder int       `json:"sort_order"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Service handles FAQ CRUD and weight tracking.
type Service struct {
	readDB  *sql.DB
	writeDB *sql.DB
}

// NewService creates a new FAQ Service.
func NewService(readDB, writeDB *sql.DB) *Service {
	return &Service{readDB: readDB, writeDB: writeDB}
}

// RecordQuestion records a user query. If a similar question already exists
// for the product (case-insensitive, trimmed match), its weight is incremented.
// Otherwise a new entry is created with weight 1.
func (s *Service) RecordQuestion(productID, question string) error {
	question = strings.TrimSpace(question)
	if question == "" || productID == "" {
		return nil
	}
	normalized := normalizeQuestion(question)
	if normalized == "" {
		return nil
	}

	// Try to find existing entry with same normalized question for this product
	var existingID string
	err := s.writeDB.QueryRow(
		`SELECT id FROM faq_entries WHERE product_id = ? AND normalized = ?`,
		productID, normalized,
	).Scan(&existingID)

	if err == nil {
		// Found existing — increment weight
		_, err = s.writeDB.Exec(
			`UPDATE faq_entries SET weight = weight + 1, updated_at = CURRENT_TIMESTAMP WHERE id = ?`,
			existingID,
		)
		return err
	}
	if err != sql.ErrNoRows {
		return fmt.Errorf("faq lookup failed: %w", err)
	}

	// New entry — get next sort_order
	var maxOrder int
	_ = s.writeDB.QueryRow(
		`SELECT COALESCE(MAX(sort_order), 0) FROM faq_entries WHERE product_id = ?`,
		productID,
	).Scan(&maxOrder)

	id, err := generateID()
	if err != nil {
		return err
	}
	_, err = s.writeDB.Exec(
		`INSERT INTO faq_entries (id, product_id, question, normalized, weight, sort_order, created_at, updated_at)
		 VALUES (?, ?, ?, ?, 1, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`,
		id, productID, question, normalized, maxOrder+1,
	)
	return err
}

// Create manually creates a FAQ entry (admin operation).
func (s *Service) Create(productID, question string) (*Entry, error) {
	question = strings.TrimSpace(question)
	if question == "" {
		return nil, fmt.Errorf("question cannot be empty")
	}
	if productID == "" {
		return nil, fmt.Errorf("product_id is required")
	}

	normalized := normalizeQuestion(question)

	// Check for duplicate
	var existingID string
	err := s.writeDB.QueryRow(
		`SELECT id FROM faq_entries WHERE product_id = ? AND normalized = ?`,
		productID, normalized,
	).Scan(&existingID)
	if err == nil {
		return nil, fmt.Errorf("该问题已存在")
	}

	var maxOrder int
	_ = s.writeDB.QueryRow(
		`SELECT COALESCE(MAX(sort_order), 0) FROM faq_entries WHERE product_id = ?`,
		productID,
	).Scan(&maxOrder)

	id, err := generateID()
	if err != nil {
		return nil, err
	}
	now := time.Now()
	_, err = s.writeDB.Exec(
		`INSERT INTO faq_entries (id, product_id, question, normalized, weight, sort_order, created_at, updated_at)
		 VALUES (?, ?, ?, ?, 2, ?, ?, ?)`,
		id, productID, question, normalized, maxOrder+1, now, now,
	)
	if err != nil {
		return nil, err
	}
	return &Entry{
		ID: id, ProductID: productID, Question: question,
		Weight: 2, SortOrder: maxOrder + 1, CreatedAt: now, UpdatedAt: now,
	}, nil
}

// TopFAQ returns the top N FAQ entries for a product.
// Uses sort_order first (admin-defined order), then weight as tiebreaker.
// Only entries with weight >= 2 are shown to users.
func (s *Service) TopFAQ(productID string, limit int) ([]Entry, error) {
	if productID == "" {
		return nil, fmt.Errorf("product_id is required")
	}
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	rows, err := s.readDB.Query(
		`SELECT id, product_id, question, weight, sort_order, created_at, updated_at
		 FROM faq_entries
		 WHERE product_id = ? AND weight >= 2
		 ORDER BY sort_order ASC, weight DESC
		 LIMIT ?`,
		productID, limit,
	)
	if err != nil {
		return nil, fmt.Errorf("faq query failed: %w", err)
	}
	defer rows.Close()

	var entries []Entry
	for rows.Next() {
		var e Entry
		if err := rows.Scan(&e.ID, &e.ProductID, &e.Question, &e.Weight, &e.SortOrder, &e.CreatedAt, &e.UpdatedAt); err != nil {
			return nil, err
		}
		entries = append(entries, e)
	}
	return entries, rows.Err()
}

// ListAll returns all FAQ entries for a product (admin view, no weight filter).
func (s *Service) ListAll(productID string) ([]Entry, error) {
	if productID == "" {
		return nil, fmt.Errorf("product_id is required")
	}
	rows, err := s.readDB.Query(
		`SELECT id, product_id, question, weight, sort_order, created_at, updated_at
		 FROM faq_entries
		 WHERE product_id = ?
		 ORDER BY sort_order ASC, weight DESC`,
		productID,
	)
	if err != nil {
		return nil, fmt.Errorf("faq query failed: %w", err)
	}
	defer rows.Close()

	var entries []Entry
	for rows.Next() {
		var e Entry
		if err := rows.Scan(&e.ID, &e.ProductID, &e.Question, &e.Weight, &e.SortOrder, &e.CreatedAt, &e.UpdatedAt); err != nil {
			return nil, err
		}
		entries = append(entries, e)
	}
	return entries, rows.Err()
}

// Delete removes a FAQ entry by ID (admin operation).
func (s *Service) Delete(id string) error {
	_, err := s.writeDB.Exec(`DELETE FROM faq_entries WHERE id = ?`, id)
	return err
}

// UpdateQuestion updates the display question text for a FAQ entry (admin operation).
func (s *Service) UpdateQuestion(id, question string) error {
	question = strings.TrimSpace(question)
	if question == "" {
		return fmt.Errorf("question cannot be empty")
	}
	normalized := normalizeQuestion(question)
	_, err := s.writeDB.Exec(
		`UPDATE faq_entries SET question = ?, normalized = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?`,
		question, normalized, id,
	)
	return err
}

// Reorder updates the sort_order for a list of FAQ entry IDs.
// The order of IDs in the slice determines the new sort_order (0-based).
func (s *Service) Reorder(ids []string) error {
	if len(ids) == 0 {
		return nil
	}
	tx, err := s.writeDB.Begin()
	if err != nil {
		return err
	}
	stmt, err := tx.Prepare(`UPDATE faq_entries SET sort_order = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?`)
	if err != nil {
		tx.Rollback()
		return err
	}
	defer stmt.Close()
	for i, id := range ids {
		if _, err := stmt.Exec(i+1, id); err != nil {
			tx.Rollback()
			return err
		}
	}
	return tx.Commit()
}

// normalizeQuestion produces a canonical form for deduplication.
func normalizeQuestion(q string) string {
	q = strings.ToLower(strings.TrimSpace(q))
	fields := strings.Fields(q)
	q = strings.Join(fields, " ")
	q = strings.TrimRight(q, "?？。.!！")
	return q
}

func generateID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("failed to generate ID: %w", err)
	}
	return hex.EncodeToString(b), nil
}

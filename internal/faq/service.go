// Package faq provides FAQ (frequently asked questions) management per product.
// Each user query is recorded and similar questions are merged by increasing weight.
// The top-weighted entries surface as FAQ for the chat UI.
// Admins can manually add, reorder, edit, and delete FAQ entries.
package faq

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
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

// EmbedFunc generates an embedding vector for a text string.
type EmbedFunc func(text string) ([]float64, error)

// SimilarityFunc computes similarity between two vectors (e.g. cosine similarity).
type SimilarityFunc func(a, b []float64) float64

// SimilarityThreshold is the minimum cosine similarity to consider two questions as duplicates.
const SimilarityThreshold = 0.90

// Service handles FAQ CRUD and weight tracking.
type Service struct {
	readDB    *sql.DB
	writeDB   *sql.DB
	embedFn   EmbedFunc
	similarFn SimilarityFunc
}

// NewService creates a new FAQ Service.
func NewService(readDB, writeDB *sql.DB) *Service {
	return &Service{readDB: readDB, writeDB: writeDB}
}

// SetEmbedding configures embedding-based similarity deduplication.
// When set, RecordQuestion will use vector similarity to merge semantically
// similar questions even if their text differs.
func (s *Service) SetEmbedding(embed EmbedFunc, similarity SimilarityFunc) {
	s.embedFn = embed
	s.similarFn = similarity
}

// RecordQuestion records a user query. If a similar question already exists
// for the product (exact normalized match or embedding similarity >= threshold),
// its weight is incremented. Otherwise a new entry is created with weight 1.
func (s *Service) RecordQuestion(productID, question string) error {
	question = strings.TrimSpace(question)
	if question == "" || productID == "" {
		return nil
	}
	normalized := normalizeQuestion(question)
	if normalized == "" {
		return nil
	}

	// Level 1: exact normalized match
	var existingID string
	err := s.writeDB.QueryRow(
		`SELECT id FROM faq_entries WHERE product_id = ? AND normalized = ?`,
		productID, normalized,
	).Scan(&existingID)

	if err == nil {
		_, err = s.writeDB.Exec(
			`UPDATE faq_entries SET weight = weight + 1, updated_at = CURRENT_TIMESTAMP WHERE id = ?`,
			existingID,
		)
		return err
	}
	if err != sql.ErrNoRows {
		return fmt.Errorf("faq lookup failed: %w", err)
	}

	// Level 2: embedding similarity match (if configured)
	if matchID := s.findSimilarByEmbedding(productID, question); matchID != "" {
		_, err = s.writeDB.Exec(
			`UPDATE faq_entries SET weight = weight + 1, updated_at = CURRENT_TIMESTAMP WHERE id = ?`,
			matchID,
		)
		return err
	}

	// No match — create new entry
	var maxOrder int
	_ = s.writeDB.QueryRow(
		`SELECT COALESCE(MAX(sort_order), 0) FROM faq_entries WHERE product_id = ?`,
		productID,
	).Scan(&maxOrder)

	id, err := generateID()
	if err != nil {
		return err
	}

	// Pre-compute embedding for the new entry
	var embJSON []byte
	if s.embedFn != nil {
		if vec, embErr := s.embedFn(question); embErr == nil && len(vec) > 0 {
			embJSON, _ = json.Marshal(vec)
		}
	}

	_, err = s.writeDB.Exec(
		`INSERT INTO faq_entries (id, product_id, question, normalized, weight, sort_order, embedding, created_at, updated_at)
		 VALUES (?, ?, ?, ?, 1, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`,
		id, productID, question, normalized, maxOrder+1, nullableBytes(embJSON),
	)
	return err
}

// findSimilarByEmbedding searches existing FAQ entries for the product using
// vector cosine similarity. Returns the ID of the best match above threshold,
// or empty string if none found.
func (s *Service) findSimilarByEmbedding(productID, question string) string {
	if s.embedFn == nil || s.similarFn == nil {
		return ""
	}

	queryVec, err := s.embedFn(question)
	if err != nil || len(queryVec) == 0 {
		log.Printf("[FAQ] embedding failed for similarity check: %v", err)
		return ""
	}

	// Load existing entries with their cached embeddings (cap scan to prevent unbounded memory use)
	rows, err := s.readDB.Query(
		`SELECT id, question, embedding FROM faq_entries WHERE product_id = ? ORDER BY weight DESC LIMIT 500`,
		productID,
	)
	if err != nil {
		return ""
	}
	defer rows.Close()

	var bestID string
	var bestScore float64

	for rows.Next() {
		var id, q string
		var embData sql.NullString
		if err := rows.Scan(&id, &q, &embData); err != nil {
			continue
		}

		var entryVec []float64
		if embData.Valid && embData.String != "" {
			if err := json.Unmarshal([]byte(embData.String), &entryVec); err == nil && len(entryVec) > 0 {
				// Use cached embedding
				score := s.similarFn(queryVec, entryVec)
				if score > bestScore {
					bestScore = score
					bestID = id
				}
				continue
			}
		}

		// No cached embedding — compute and cache it
		entryVec, embErr := s.embedFn(q)
		if embErr != nil || len(entryVec) == 0 {
			continue
		}
		// Cache the embedding for future use
		if embJSON, err := json.Marshal(entryVec); err == nil {
			s.writeDB.Exec(
				`UPDATE faq_entries SET embedding = ? WHERE id = ?`,
				string(embJSON), id,
			)
		}

		score := s.similarFn(queryVec, entryVec)
		if score > bestScore {
			bestScore = score
			bestID = id
		}
	}

	if bestScore >= SimilarityThreshold {
		log.Printf("[FAQ] semantic merge: %.4f similarity for product=%s", bestScore, productID)
		return bestID
	}
	return ""
}

// nullableBytes returns a sql.NullString for optional embedding data.
func nullableBytes(data []byte) sql.NullString {
	if len(data) == 0 {
		return sql.NullString{}
	}
	return sql.NullString{String: string(data), Valid: true}
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

	// Pre-compute embedding for the new entry
	var embVal sql.NullString
	if s.embedFn != nil {
		if vec, embErr := s.embedFn(question); embErr == nil && len(vec) > 0 {
			if data, jErr := json.Marshal(vec); jErr == nil {
				embVal = sql.NullString{String: string(data), Valid: true}
			}
		}
	}

	now := time.Now()
	_, err = s.writeDB.Exec(
		`INSERT INTO faq_entries (id, product_id, question, normalized, weight, sort_order, embedding, created_at, updated_at)
		 VALUES (?, ?, ?, ?, 2, ?, ?, ?, ?)`,
		id, productID, question, normalized, maxOrder+1, embVal, now, now,
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
// Also recomputes the cached embedding if embedding is configured.
func (s *Service) UpdateQuestion(id, question string) error {
	question = strings.TrimSpace(question)
	if question == "" {
		return fmt.Errorf("question cannot be empty")
	}
	normalized := normalizeQuestion(question)

	// Recompute embedding for the updated question text
	var embVal sql.NullString
	if s.embedFn != nil {
		if vec, err := s.embedFn(question); err == nil && len(vec) > 0 {
			if data, err := json.Marshal(vec); err == nil {
				embVal = sql.NullString{String: string(data), Valid: true}
			}
		}
	}

	_, err := s.writeDB.Exec(
		`UPDATE faq_entries SET question = ?, normalized = ?, embedding = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?`,
		question, normalized, embVal, id,
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

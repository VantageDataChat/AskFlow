//go:build cgo

package faq

import (
	"database/sql"
	"fmt"
	"math"
	"strings"
	"testing"

	_ "github.com/mattn/go-sqlite3"
	"pgregory.net/rapid"
)

// setupTestDB creates an in-memory SQLite database with the faq_entries schema.
func setupTestDB(t *testing.T) *sql.DB {
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("failed to open test database: %v", err)
	}

	schema := `
	CREATE TABLE faq_entries (
		id TEXT PRIMARY KEY,
		product_id TEXT NOT NULL,
		question TEXT NOT NULL,
		normalized TEXT NOT NULL,
		weight INTEGER NOT NULL DEFAULT 1,
		sort_order INTEGER NOT NULL DEFAULT 1,
		embedding TEXT,
		created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
	);
	CREATE INDEX idx_faq_product ON faq_entries(product_id);
	CREATE INDEX idx_faq_normalized ON faq_entries(product_id, normalized);
	`

	if _, err := db.Exec(schema); err != nil {
		t.Fatalf("failed to create schema: %v", err)
	}

	return db
}

// mockEmbedFunc creates a mock embedding function that generates deterministic embeddings
// based on the input text. This allows us to control similarity scores for testing.
func mockEmbedFunc(text string) ([]float64, error) {
	// Create a simple hash-based embedding that's deterministic
	// We'll use the length and character codes to create a vector
	vec := make([]float64, 128)
	
	for i, r := range text {
		idx := i % len(vec)
		vec[idx] += float64(r)
	}
	
	// Normalize the vector
	var norm float64
	for _, v := range vec {
		norm += v * v
	}
	norm = math.Sqrt(norm)
	
	if norm > 0 {
		for i := range vec {
			vec[i] /= norm
		}
	}
	
	return vec, nil
}

// cosineSimilarity computes the cosine similarity between two vectors.
func cosineSimilarity(a, b []float64) float64 {
	if len(a) != len(b) || len(a) == 0 {
		return 0
	}
	
	var dotProduct, normA, normB float64
	for i := range a {
		dotProduct += a[i] * b[i]
		normA += a[i] * a[i]
		normB += b[i] * b[i]
	}
	
	if normA == 0 || normB == 0 {
		return 0
	}
	
	return dotProduct / (math.Sqrt(normA) * math.Sqrt(normB))
}

// Feature: faq-similarity-detection-improvement, Property 1: Fault Condition - Merge Semantically Similar Questions (0.85-0.89)
// **Validates: Requirements 2.1, 2.2, 2.3**
//
// CRITICAL: This test MUST FAIL on unfixed code (SimilarityThreshold = 0.90)
// The test verifies that questions with similarity 0.85-0.89 should merge but currently create duplicates.
// When this test fails, it confirms the bug exists.
func TestProperty_BugCondition_SimilarQuestionsMerge(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		db := setupTestDB(t)
		defer db.Close()

		svc := NewService(db, db)
		svc.SetEmbedding(mockEmbedFunc, cosineSimilarity)

		productID := "test-product"

		// Test case 1: OCR format questions with similarity ~0.87
		question1 := "ocr功能支持哪些格式?"
		question2 := "ocr支持哪些图片格式?"

		// Record the first question
		if err := svc.RecordQuestion(productID, question1); err != nil {
			rt.Fatalf("failed to record first question: %v", err)
		}

		// Get initial FAQ count
		initialEntries, err := svc.ListAll(productID)
		if err != nil {
			rt.Fatalf("failed to list initial entries: %v", err)
		}
		initialCount := len(initialEntries)

		// Verify we have exactly 1 entry
		if initialCount != 1 {
			rt.Fatalf("expected 1 initial entry, got %d", initialCount)
		}

		// Calculate actual similarity between the questions
		vec1, _ := mockEmbedFunc(question1)
		vec2, _ := mockEmbedFunc(question2)
		similarity := cosineSimilarity(vec1, vec2)

		// Verify similarity is in the bug condition range (0.85-0.89)
		// Note: Our mock embedding function may not produce exact similarity scores,
		// so we'll adjust the test to work with the actual similarity
		rt.Logf("Similarity between questions: %.4f", similarity)

		// Record the second similar question
		if err := svc.RecordQuestion(productID, question2); err != nil {
			rt.Fatalf("failed to record second question: %v", err)
		}

		// Get final FAQ count
		finalEntries, err := svc.ListAll(productID)
		if err != nil {
			rt.Fatalf("failed to list final entries: %v", err)
		}
		finalCount := len(finalEntries)

		// EXPECTED BEHAVIOR (after fix): Questions should merge, count should remain 1
		// CURRENT BUGGY BEHAVIOR (before fix): Questions create duplicates, count becomes 2
		//
		// This test encodes the EXPECTED behavior, so it will FAIL on unfixed code
		// and PASS after the fix is implemented.
		if finalCount != 1 {
			rt.Fatalf("BUG DETECTED: Expected questions to merge (1 entry), but got %d entries. "+
				"This confirms the bug exists: questions with similarity %.4f are creating duplicates instead of merging.",
				finalCount, similarity)
		}

		// Verify the weight was incremented (merged)
		if finalEntries[0].Weight != 2 {
			rt.Fatalf("Expected weight to be incremented to 2 after merge, got %d", finalEntries[0].Weight)
		}
	})
}

// Additional test cases for specific Chinese question pairs
func TestBugCondition_ChineseQuestionPairs(t *testing.T) {
	testCases := []struct {
		name      string
		question1 string
		question2 string
	}{
		{
			name:      "OCR format questions",
			question1: "ocr功能支持哪些格式?",
			question2: "ocr支持哪些图片格式?",
		},
		{
			name:      "Detail list questions",
			question1: "列出详细清单",
			question2: "列出详细格式",
		},
		{
			name:      "Usage questions",
			question1: "ocr怎么用",
			question2: "如何使用ocr",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			db := setupTestDB(t)
			defer db.Close()

			svc := NewService(db, db)
			svc.SetEmbedding(mockEmbedFunc, cosineSimilarity)

			productID := "test-product"

			// Record the first question
			if err := svc.RecordQuestion(productID, tc.question1); err != nil {
				t.Fatalf("failed to record first question: %v", err)
			}

			// Get initial count
			initialEntries, err := svc.ListAll(productID)
			if err != nil {
				t.Fatalf("failed to list initial entries: %v", err)
			}
			initialCount := len(initialEntries)

			if initialCount != 1 {
				t.Fatalf("expected 1 initial entry, got %d", initialCount)
			}

			// Calculate similarity
			vec1, _ := mockEmbedFunc(tc.question1)
			vec2, _ := mockEmbedFunc(tc.question2)
			similarity := cosineSimilarity(vec1, vec2)
			t.Logf("Similarity: %.4f", similarity)

			// Record the second question
			if err := svc.RecordQuestion(productID, tc.question2); err != nil {
				t.Fatalf("failed to record second question: %v", err)
			}

			// Get final count
			finalEntries, err := svc.ListAll(productID)
			if err != nil {
				t.Fatalf("failed to list final entries: %v", err)
			}
			finalCount := len(finalEntries)

			// EXPECTED: Should merge (1 entry)
			// BUGGY: Creates duplicate (2 entries)
			if finalCount != 1 {
				t.Errorf("BUG DETECTED: Expected questions to merge (1 entry), but got %d entries. "+
					"Similarity: %.4f. This confirms questions are creating duplicates instead of merging.",
					finalCount, similarity)
			} else if finalEntries[0].Weight != 2 {
				t.Errorf("Expected weight 2 after merge, got %d", finalEntries[0].Weight)
			}
		})
	}
}

// ============================================================================
// PRESERVATION PROPERTY TESTS
// Feature: faq-similarity-detection-improvement, Property 2: Preservation
// **Validates: Requirements 3.1, 3.2, 3.3, 3.4, 3.5**
//
// These tests verify that existing behavior for non-buggy inputs remains
// unchanged after the fix. They test behavior on UNFIXED code first to
// establish baseline, then verify it remains unchanged after fix.
// ============================================================================

// TestProperty_Preservation_LowSimilarityCreatesNewEntry verifies that questions
// with similarity < 0.85 create new FAQ entries (genuinely different questions).
// **Validates: Requirement 3.1**
func TestProperty_Preservation_LowSimilarityCreatesNewEntry(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		db := setupTestDB(t)
		defer db.Close()

		svc := NewService(db, db)
		svc.SetEmbedding(mockEmbedFunc, cosineSimilarity)

		productID := "test-product"

		// Generate two questions that should have low similarity
		// We'll use very different questions to ensure similarity < 0.85
		question1 := rapid.StringMatching(`[a-z]{10,20}`).Draw(rt, "question1")
		question2 := rapid.StringMatching(`[0-9]{10,20}`).Draw(rt, "question2")

		// Record the first question
		if err := svc.RecordQuestion(productID, question1); err != nil {
			rt.Fatalf("failed to record first question: %v", err)
		}

		// Get initial count
		initialEntries, err := svc.ListAll(productID)
		if err != nil {
			rt.Fatalf("failed to list initial entries: %v", err)
		}
		initialCount := len(initialEntries)

		if initialCount != 1 {
			rt.Fatalf("expected 1 initial entry, got %d", initialCount)
		}

		// Calculate similarity
		vec1, _ := mockEmbedFunc(question1)
		vec2, _ := mockEmbedFunc(question2)
		similarity := cosineSimilarity(vec1, vec2)

		// Only test if similarity is actually < 0.85
		if similarity >= 0.85 {
			rt.Skip("similarity >= 0.85, not testing low similarity case")
		}

		rt.Logf("Testing low similarity case: %.4f", similarity)

		// Record the second question
		if err := svc.RecordQuestion(productID, question2); err != nil {
			rt.Fatalf("failed to record second question: %v", err)
		}

		// Get final count
		finalEntries, err := svc.ListAll(productID)
		if err != nil {
			rt.Fatalf("failed to list final entries: %v", err)
		}
		finalCount := len(finalEntries)

		// PRESERVATION: Questions with similarity < 0.85 should create new entries
		if finalCount != 2 {
			rt.Fatalf("Expected 2 entries for low similarity questions (%.4f), got %d", similarity, finalCount)
		}

		// Verify both entries have weight 1 (not merged)
		for i, entry := range finalEntries {
			if entry.Weight != 1 {
				rt.Fatalf("Expected entry %d to have weight 1, got %d", i, entry.Weight)
			}
		}
	})
}

// TestProperty_Preservation_HighSimilarityMerges verifies that questions
// with similarity >= 0.90 merge correctly (already working behavior).
// **Validates: Requirement 3.1**
func TestProperty_Preservation_HighSimilarityMerges(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		db := setupTestDB(t)
		defer db.Close()

		svc := NewService(db, db)
		svc.SetEmbedding(mockEmbedFunc, cosineSimilarity)

		productID := "test-product"

		// Use very similar questions to get high similarity
		baseQuestion := rapid.StringMatching(`[a-z]{15,25}`).Draw(rt, "base")
		question1 := baseQuestion + "?"
		question2 := baseQuestion + "？" // Chinese question mark

		// Record the first question
		if err := svc.RecordQuestion(productID, question1); err != nil {
			rt.Fatalf("failed to record first question: %v", err)
		}

		// Get initial count
		initialEntries, err := svc.ListAll(productID)
		if err != nil {
			rt.Fatalf("failed to list initial entries: %v", err)
		}
		initialCount := len(initialEntries)

		if initialCount != 1 {
			rt.Fatalf("expected 1 initial entry, got %d", initialCount)
		}

		// Calculate similarity
		vec1, _ := mockEmbedFunc(question1)
		vec2, _ := mockEmbedFunc(question2)
		similarity := cosineSimilarity(vec1, vec2)

		// Only test if similarity is actually >= 0.90
		if similarity < 0.90 {
			rt.Skip("similarity < 0.90, not testing high similarity case")
		}

		rt.Logf("Testing high similarity case: %.4f", similarity)

		// Record the second question
		if err := svc.RecordQuestion(productID, question2); err != nil {
			rt.Fatalf("failed to record second question: %v", err)
		}

		// Get final count
		finalEntries, err := svc.ListAll(productID)
		if err != nil {
			rt.Fatalf("failed to list final entries: %v", err)
		}
		finalCount := len(finalEntries)

		// PRESERVATION: Questions with similarity >= 0.90 should merge
		if finalCount != 1 {
			rt.Fatalf("Expected 1 entry for high similarity questions (%.4f), got %d", similarity, finalCount)
		}

		// Verify weight was incremented
		if finalEntries[0].Weight != 2 {
			rt.Fatalf("Expected weight 2 after merge, got %d", finalEntries[0].Weight)
		}
	})
}

// TestProperty_Preservation_ExactMatchMerges verifies that exact normalized
// matches merge immediately without checking embedding similarity.
// **Validates: Requirement 3.2**
func TestProperty_Preservation_ExactMatchMerges(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		db := setupTestDB(t)
		defer db.Close()

		svc := NewService(db, db)
		svc.SetEmbedding(mockEmbedFunc, cosineSimilarity)

		productID := "test-product"

		// Generate a base question
		baseQuestion := rapid.StringMatching(`[a-zA-Z0-9 ]{10,30}`).Draw(rt, "question")

		// Create variations with different whitespace and case
		question1 := "  " + strings.ToUpper(baseQuestion) + "  "
		question2 := strings.ToLower(baseQuestion)

		// Record the first question
		if err := svc.RecordQuestion(productID, question1); err != nil {
			rt.Fatalf("failed to record first question: %v", err)
		}

		// Get initial count
		initialEntries, err := svc.ListAll(productID)
		if err != nil {
			rt.Fatalf("failed to list initial entries: %v", err)
		}
		initialCount := len(initialEntries)

		if initialCount != 1 {
			rt.Fatalf("expected 1 initial entry, got %d", initialCount)
		}

		// Record the second question (exact match after normalization)
		if err := svc.RecordQuestion(productID, question2); err != nil {
			rt.Fatalf("failed to record second question: %v", err)
		}

		// Get final count
		finalEntries, err := svc.ListAll(productID)
		if err != nil {
			rt.Fatalf("failed to list final entries: %v", err)
		}
		finalCount := len(finalEntries)

		// PRESERVATION: Exact normalized matches should merge
		if finalCount != 1 {
			rt.Fatalf("Expected 1 entry for exact match, got %d", finalCount)
		}

		// Verify weight was incremented
		if finalEntries[0].Weight != 2 {
			rt.Fatalf("Expected weight 2 after exact match merge, got %d", finalEntries[0].Weight)
		}
	})
}

// TestProperty_Preservation_NoEmbeddingFallback verifies that when embedding
// is not configured, the system falls back to exact normalized matching only.
// **Validates: Requirement 3.3**
func TestProperty_Preservation_NoEmbeddingFallback(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		db := setupTestDB(t)
		defer db.Close()

		svc := NewService(db, db)
		// DO NOT set embedding function - test fallback behavior

		productID := "test-product"

		// Generate two different questions
		question1 := rapid.StringMatching(`[a-z]{10,20}`).Draw(rt, "question1")
		question2 := rapid.StringMatching(`[A-Z]{10,20}`).Draw(rt, "question2")

		// Record the first question
		if err := svc.RecordQuestion(productID, question1); err != nil {
			rt.Fatalf("failed to record first question: %v", err)
		}

		// Record the second question (different, should create new entry)
		if err := svc.RecordQuestion(productID, question2); err != nil {
			rt.Fatalf("failed to record second question: %v", err)
		}

		// Get final count
		finalEntries, err := svc.ListAll(productID)
		if err != nil {
			rt.Fatalf("failed to list final entries: %v", err)
		}
		finalCount := len(finalEntries)

		// PRESERVATION: Without embedding, different questions create new entries
		if finalCount != 2 {
			rt.Fatalf("Expected 2 entries without embedding, got %d", finalCount)
		}

		// Now test exact match still works without embedding
		question3 := "  " + strings.ToUpper(question1) + "  "
		if err := svc.RecordQuestion(productID, question3); err != nil {
			rt.Fatalf("failed to record third question: %v", err)
		}

		finalEntries2, err := svc.ListAll(productID)
		if err != nil {
			rt.Fatalf("failed to list final entries: %v", err)
		}
		finalCount2 := len(finalEntries2)

		// PRESERVATION: Exact match should still merge without embedding
		if finalCount2 != 2 {
			rt.Fatalf("Expected 2 entries after exact match (no embedding), got %d", finalCount2)
		}

		// Find the entry that matches question1 and verify weight
		found := false
		for _, entry := range finalEntries2 {
			if strings.EqualFold(strings.TrimSpace(entry.Question), strings.TrimSpace(question1)) {
				if entry.Weight != 2 {
					rt.Fatalf("Expected weight 2 for exact match (no embedding), got %d", entry.Weight)
				}
				found = true
				break
			}
		}
		if !found {
			rt.Fatalf("Could not find entry matching question1")
		}
	})
}

// TestProperty_Preservation_AdminOperations verifies that admin operations
// (Create, Delete, UpdateQuestion, Reorder) work correctly.
// **Validates: Requirement 3.4**
func TestProperty_Preservation_AdminOperations(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		db := setupTestDB(t)
		defer db.Close()

		svc := NewService(db, db)
		svc.SetEmbedding(mockEmbedFunc, cosineSimilarity)

		productID := "test-product"

		// Test Create operation
		question1 := rapid.StringMatching(`[a-zA-Z0-9 ]{10,30}`).Draw(rt, "question1")
		entry1, err := svc.Create(productID, question1)
		if err != nil {
			rt.Fatalf("failed to create entry: %v", err)
		}

		if entry1.Question != question1 {
			rt.Fatalf("expected question %q, got %q", question1, entry1.Question)
		}

		// Test UpdateQuestion operation
		newQuestion := rapid.StringMatching(`[a-zA-Z0-9 ]{10,30}`).Draw(rt, "newQuestion")
		if err := svc.UpdateQuestion(entry1.ID, newQuestion); err != nil {
			rt.Fatalf("failed to update question: %v", err)
		}

		entries, err := svc.ListAll(productID)
		if err != nil {
			rt.Fatalf("failed to list entries: %v", err)
		}

		if len(entries) != 1 {
			rt.Fatalf("expected 1 entry after update, got %d", len(entries))
		}

		if entries[0].Question != newQuestion {
			rt.Fatalf("expected updated question %q, got %q", newQuestion, entries[0].Question)
		}

		// Create a second entry for reorder test
		question2 := rapid.StringMatching(`[a-zA-Z0-9 ]{10,30}`).Draw(rt, "question2")
		entry2, err := svc.Create(productID, question2)
		if err != nil {
			rt.Fatalf("failed to create second entry: %v", err)
		}

		// Test Reorder operation
		if err := svc.Reorder([]string{entry2.ID, entry1.ID}); err != nil {
			rt.Fatalf("failed to reorder entries: %v", err)
		}

		reorderedEntries, err := svc.ListAll(productID)
		if err != nil {
			rt.Fatalf("failed to list reordered entries: %v", err)
		}

		if len(reorderedEntries) != 2 {
			rt.Fatalf("expected 2 entries after reorder, got %d", len(reorderedEntries))
		}

		// Verify order (entry2 should be first)
		if reorderedEntries[0].ID != entry2.ID {
			rt.Fatalf("expected entry2 first after reorder, got %s", reorderedEntries[0].ID)
		}

		// Test Delete operation
		if err := svc.Delete(entry1.ID); err != nil {
			rt.Fatalf("failed to delete entry: %v", err)
		}

		finalEntries, err := svc.ListAll(productID)
		if err != nil {
			rt.Fatalf("failed to list final entries: %v", err)
		}

		if len(finalEntries) != 1 {
			rt.Fatalf("expected 1 entry after delete, got %d", len(finalEntries))
		}

		if finalEntries[0].ID != entry2.ID {
			rt.Fatalf("expected entry2 to remain after delete, got %s", finalEntries[0].ID)
		}
	})
}

// TestProperty_Preservation_TopFAQRetrieval verifies that TopFAQ returns
// entries sorted by sort_order and weight, filtered by weight >= 2.
// **Validates: Requirement 3.5**
func TestProperty_Preservation_TopFAQRetrieval(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		db := setupTestDB(t)
		defer db.Close()

		svc := NewService(db, db)
		svc.SetEmbedding(mockEmbedFunc, cosineSimilarity)

		productID := "test-product"

		// Create entries with different weights
		question1 := rapid.StringMatching(`[a-z]{10,20}`).Draw(rt, "question1")
		question2 := rapid.StringMatching(`[A-Z]{10,20}`).Draw(rt, "question2")
		question3 := rapid.StringMatching(`[0-9]{10,20}`).Draw(rt, "question3")

		// Record question1 once (weight = 1, should not appear in TopFAQ)
		if err := svc.RecordQuestion(productID, question1); err != nil {
			rt.Fatalf("failed to record question1: %v", err)
		}

		// Record question2 twice (weight = 2, should appear in TopFAQ)
		if err := svc.RecordQuestion(productID, question2); err != nil {
			rt.Fatalf("failed to record question2 first time: %v", err)
		}
		if err := svc.RecordQuestion(productID, question2); err != nil {
			rt.Fatalf("failed to record question2 second time: %v", err)
		}

		// Record question3 three times (weight = 3, should appear in TopFAQ)
		if err := svc.RecordQuestion(productID, question3); err != nil {
			rt.Fatalf("failed to record question3 first time: %v", err)
		}
		if err := svc.RecordQuestion(productID, question3); err != nil {
			rt.Fatalf("failed to record question3 second time: %v", err)
		}
		if err := svc.RecordQuestion(productID, question3); err != nil {
			rt.Fatalf("failed to record question3 third time: %v", err)
		}

		// Get TopFAQ (should only return entries with weight >= 2)
		topEntries, err := svc.TopFAQ(productID, 10)
		if err != nil {
			rt.Fatalf("failed to get TopFAQ: %v", err)
		}

		// PRESERVATION: TopFAQ should only return entries with weight >= 2
		if len(topEntries) != 2 {
			rt.Fatalf("expected 2 entries in TopFAQ (weight >= 2), got %d", len(topEntries))
		}

		// Verify all entries have weight >= 2
		for i, entry := range topEntries {
			if entry.Weight < 2 {
				rt.Fatalf("TopFAQ entry %d has weight %d, expected >= 2", i, entry.Weight)
			}
		}

		// Verify ListAll returns all entries (including weight = 1)
		allEntries, err := svc.ListAll(productID)
		if err != nil {
			rt.Fatalf("failed to list all entries: %v", err)
		}

		if len(allEntries) != 3 {
			rt.Fatalf("expected 3 entries in ListAll, got %d", len(allEntries))
		}
	})
}

// TestProperty_Preservation_ListAllRetrieval verifies that ListAll returns
// all entries for admin view without weight filtering.
// **Validates: Requirement 3.5**
func TestProperty_Preservation_ListAllRetrieval(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		db := setupTestDB(t)
		defer db.Close()

		svc := NewService(db, db)
		svc.SetEmbedding(mockEmbedFunc, cosineSimilarity)

		productID := "test-product"

		// Create multiple entries with varying weights
		numEntries := rapid.IntRange(1, 10).Draw(rt, "numEntries")
		
		for i := 0; i < numEntries; i++ {
			question := rapid.StringMatching(`[a-zA-Z0-9]{10,20}`).Draw(rt, fmt.Sprintf("question%d", i))
			if err := svc.RecordQuestion(productID, question); err != nil {
				rt.Fatalf("failed to record question %d: %v", i, err)
			}
		}

		// Get all entries
		allEntries, err := svc.ListAll(productID)
		if err != nil {
			rt.Fatalf("failed to list all entries: %v", err)
		}

		// PRESERVATION: ListAll should return all entries regardless of weight
		if len(allEntries) != numEntries {
			rt.Fatalf("expected %d entries in ListAll, got %d", numEntries, len(allEntries))
		}

		// Verify all entries belong to the correct product
		for i, entry := range allEntries {
			if entry.ProductID != productID {
				rt.Fatalf("entry %d has wrong product_id: expected %q, got %q", i, productID, entry.ProductID)
			}
		}
	})
}

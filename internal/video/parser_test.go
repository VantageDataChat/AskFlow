package video

import (
	"encoding/json"
	"testing"
)

func TestParseWhisperOutput_ValidJSON(t *testing.T) {
	input := []byte(`{
		"segments": [
			{"start": 0.0, "end": 5.2, "text": "Hello world"},
			{"start": 5.2, "end": 10.1, "text": "This is a test"}
		]
	}`)

	segments, err := ParseWhisperOutput(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(segments) != 2 {
		t.Fatalf("expected 2 segments, got %d", len(segments))
	}
	if segments[0].Start != 0.0 || segments[0].End != 5.2 || segments[0].Text != "Hello world" {
		t.Errorf("segment[0] mismatch: %+v", segments[0])
	}
	if segments[1].Start != 5.2 || segments[1].End != 10.1 || segments[1].Text != "This is a test" {
		t.Errorf("segment[1] mismatch: %+v", segments[1])
	}
}

func TestParseWhisperOutput_EmptySegments(t *testing.T) {
	input := []byte(`{"segments": []}`)

	segments, err := ParseWhisperOutput(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(segments) != 0 {
		t.Fatalf("expected 0 segments, got %d", len(segments))
	}
}

func TestParseWhisperOutput_NoSegmentsField(t *testing.T) {
	input := []byte(`{}`)

	segments, err := ParseWhisperOutput(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(segments) != 0 {
		t.Fatalf("expected 0 segments, got %d", len(segments))
	}
}

func TestParseWhisperOutput_InvalidJSON(t *testing.T) {
	input := []byte(`not valid json`)

	_, err := ParseWhisperOutput(input)
	if err == nil {
		t.Fatal("expected error for invalid JSON, got nil")
	}
}

func TestSerializeTranscript(t *testing.T) {
	segments := []TranscriptSegment{
		{Start: 0.0, End: 5.2, Text: "Hello world"},
		{Start: 5.2, End: 10.1, Text: "This is a test"},
	}

	data, err := SerializeTranscript(segments)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var result []TranscriptSegment
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("failed to unmarshal serialized output: %v", err)
	}
	if len(result) != 2 {
		t.Fatalf("expected 2 segments, got %d", len(result))
	}
	if result[0].Text != "Hello world" || result[1].Text != "This is a test" {
		t.Errorf("deserialized segments mismatch: %+v", result)
	}
}

func TestSerializeTranscript_Empty(t *testing.T) {
	data, err := SerializeTranscript([]TranscriptSegment{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(data) != "[]" {
		t.Errorf("expected '[]', got '%s'", string(data))
	}
}

func TestSerializeTranscript_Nil(t *testing.T) {
	data, err := SerializeTranscript(nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(data) != "null" {
		t.Errorf("expected 'null', got '%s'", string(data))
	}
}

func TestRoundTrip(t *testing.T) {
	original := []TranscriptSegment{
		{Start: 0.0, End: 3.5, Text: "First segment"},
		{Start: 3.5, End: 7.0, Text: "Second segment"},
		{Start: 7.0, End: 12.3, Text: "Third segment with 中文"},
	}

	data, err := SerializeTranscript(original)
	if err != nil {
		t.Fatalf("serialize error: %v", err)
	}

	// Wrap in whisper output format for ParseWhisperOutput
	whisperJSON, err := json.Marshal(map[string]interface{}{
		"segments": json.RawMessage(data),
	})
	if err != nil {
		t.Fatalf("failed to wrap in whisper format: %v", err)
	}

	restored, err := ParseWhisperOutput(whisperJSON)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	if len(restored) != len(original) {
		t.Fatalf("length mismatch: expected %d, got %d", len(original), len(restored))
	}
	for i := range original {
		if original[i] != restored[i] {
			t.Errorf("segment[%d] mismatch: expected %+v, got %+v", i, original[i], restored[i])
		}
	}
}

// Package document — audio processing pipeline: ffmpeg conversion + RapidSpeech ASR.
package document

import (
	"fmt"
	"log"
	"os"
	"path/filepath"

	"askflow/internal/errlog"
	"askflow/internal/video"
)

// processAudio handles audio file processing:
//  1. Save audio file to disk
//  2. Convert to 16kHz mono WAV via ffmpeg (if not already WAV)
//  3. Transcribe via RapidSpeech
//  4. Chunk → embed → store transcript text
//
// Falls back to storing filename as searchable text if ASR tools are not configured.
func (dm *DocumentManager) processAudio(docID, docName string, fileData []byte, productID string) error {
	log.Printf("[Audio] Starting audio processing for doc=%s file=%q", docID, docName)

	dm.mu.RLock()
	cfg := dm.videoConfig
	dm.mu.RUnlock()

	// Save audio file to disk
	uploadDir := filepath.Join(".", "data", "uploads", docID)
	audioPath := dm.findSavedFile(uploadDir)
	if audioPath == "" {
		if err := os.MkdirAll(uploadDir, 0755); err != nil {
			return fmt.Errorf("创建上传目录失败: %w", err)
		}
		safeName := sanitizeFilename(docName, docID)
		audioPath = filepath.Join(uploadDir, safeName)
		if err := os.WriteFile(audioPath, fileData, 0644); err != nil {
			return fmt.Errorf("保存音频文件失败: %w", err)
		}
	}

	// If neither ffmpeg nor RapidSpeech is configured, store filename as fallback
	if cfg.FFmpegPath == "" || cfg.RapidSpeechPath == "" || cfg.RapidSpeechModel == "" {
		log.Printf("[Audio] ASR工具未完整配置，仅存储文件名: %s", docName)
		fallbackText := fmt.Sprintf("音频文件: %s", docName)
		if err := dm.chunkEmbedStore(docID, docName, fallbackText, productID); err != nil {
			return fmt.Errorf("存储音频文件名向量失败: %w", err)
		}
		return nil
	}

	vp := video.NewParser(cfg)

	// Convert audio to 16kHz mono WAV via ffmpeg
	tempDir, err := os.MkdirTemp("", "audio-parse-*")
	if err != nil {
		return fmt.Errorf("创建临时目录失败: %w", err)
	}
	defer os.RemoveAll(tempDir)

	wavPath := filepath.Join(tempDir, "audio.wav")
	if err := vp.ExtractAudio(audioPath, wavPath); err != nil {
		errlog.Logf("[Audio] ffmpeg conversion failed doc=%s file=%q: %v", docID, docName, err)
		return fmt.Errorf("音频转换失败: %w", err)
	}

	// Transcribe via RapidSpeech
	segments, err := vp.Transcribe(wavPath)
	if err != nil {
		errlog.Logf("[Audio] transcription failed doc=%s file=%q: %v", docID, docName, err)
		return fmt.Errorf("音频转录失败: %w", err)
	}

	if len(segments) == 0 || segments[0].Text == "" {
		log.Printf("[Audio] 未识别到语音内容，存储文件名: %s", docName)
		fallbackText := fmt.Sprintf("音频文件: %s", docName)
		if err := dm.chunkEmbedStore(docID, docName, fallbackText, productID); err != nil {
			return fmt.Errorf("存储音频文件名向量失败: %w", err)
		}
		return nil
	}

	// Join transcript segments
	var fullText string
	for _, seg := range segments {
		if fullText != "" {
			fullText += " "
		}
		fullText += seg.Text
	}

	log.Printf("[Audio] 转录完成 doc=%s: %d 字符", docID, len([]rune(fullText)))

	// Chunk → embed → store
	if err := dm.chunkEmbedStore(docID, docName, fullText, productID); err != nil {
		errlog.Logf("[Audio] chunk/embed/store failed doc=%s file=%q: %v", docID, docName, err)
		return fmt.Errorf("音频转录文本存储失败: %w", err)
	}

	return nil
}

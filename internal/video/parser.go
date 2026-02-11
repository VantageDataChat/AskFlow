// Package video provides video parsing functionality including whisper transcript
// parsing, keyframe management, and serialization utilities.
package video

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"helpdesk/internal/config"
)

// TranscriptSegment 表示 whisper 输出的一个转录片段
type TranscriptSegment struct {
	Start float64 `json:"start"` // 起始时间（秒）
	End   float64 `json:"end"`   // 结束时间（秒）
	Text  string  `json:"text"`  // 转录文本
}

// Keyframe 表示从视频中提取的一个关键帧
type Keyframe struct {
	Timestamp float64 // 帧在视频中的时间（秒）
	FilePath  string  // 帧图像文件的临时路径
}

// ParseResult 视频解析结果
type ParseResult struct {
	Transcript []TranscriptSegment // 转录片段列表（可能为空）
	Keyframes  []Keyframe          // 关键帧列表
	Duration   float64             // 视频总时长（秒）
}

// whisperOutput represents the JSON structure output by whisper CLI.
type whisperOutput struct {
	Segments []TranscriptSegment `json:"segments"`
}

// ParseWhisperOutput 解析 whisper JSON 输出为 TranscriptSegment 列表
func ParseWhisperOutput(jsonData []byte) ([]TranscriptSegment, error) {
	var output whisperOutput
	if err := json.Unmarshal(jsonData, &output); err != nil {
		return nil, fmt.Errorf("whisper JSON 解析失败: %w", err)
	}
	if output.Segments == nil {
		return []TranscriptSegment{}, nil
	}
	return output.Segments, nil
}

// SerializeTranscript 将 TranscriptSegment 列表序列化为 JSON
func SerializeTranscript(segments []TranscriptSegment) ([]byte, error) {
	return json.Marshal(segments)
}

package handler

import (
	"os"
	"os/exec"
	"strings"

	"askflow/internal/config"
)

// MediaCapability represents the system's media processing capabilities.
type MediaCapability struct {
	VideoSupported       bool     `json:"video_supported"`        // whether video files can be uploaded
	AudioSupported       bool     `json:"audio_supported"`        // whether audio files can be uploaded
	VideoKeyframeEnabled bool     `json:"video_keyframe_enabled"` // whether video keyframe extraction works
	AudioTranscription   bool     `json:"audio_transcription"`    // whether audio transcription works
	MissingComponents    []string `json:"missing_components"`     // list of missing components
	Warnings             []string `json:"warnings"`               // user-friendly warnings
}

// DetectMediaCapability checks the system's media processing capabilities.
func DetectMediaCapability(cfg *config.Config) MediaCapability {
	if cfg == nil {
		return MediaCapability{
			VideoSupported:     false,
			AudioSupported:     false,
			MissingComponents:  []string{"config"},
			Warnings:           []string{"系统配置未加载"},
		}
	}

	cap := MediaCapability{
		MissingComponents: []string{},
		Warnings:          []string{},
	}

	// Check FFmpeg availability
	ffmpegPath := cfg.Video.FFmpegPath
	ffmpegAvailable := false
	if ffmpegPath != "" {
		if _, err := os.Stat(ffmpegPath); err == nil {
			// Try to execute ffmpeg -version to verify it works
			cmd := exec.Command(ffmpegPath, "-version")
			if err := cmd.Run(); err == nil {
				ffmpegAvailable = true
			}
		}
	}

	// Check RapidSpeech availability
	rapidSpeechPath := cfg.Video.RapidSpeechPath
	rapidSpeechModel := cfg.Video.RapidSpeechModel
	rapidSpeechAvailable := false
	if rapidSpeechPath != "" && rapidSpeechModel != "" {
		if _, err := os.Stat(rapidSpeechPath); err == nil {
			if _, err := os.Stat(rapidSpeechModel); err == nil {
				rapidSpeechAvailable = true
			}
		}
	}

	// Determine capabilities based on available components
	if ffmpegAvailable {
		cap.VideoSupported = true
		cap.VideoKeyframeEnabled = true
		cap.AudioSupported = true // FFmpeg can handle audio too
	} else {
		cap.MissingComponents = append(cap.MissingComponents, "ffmpeg")
	}

	if rapidSpeechAvailable {
		cap.AudioTranscription = true
	} else {
		if rapidSpeechPath == "" || rapidSpeechModel == "" {
			cap.MissingComponents = append(cap.MissingComponents, "rapidspeech")
		}
	}

	// Generate user-friendly warnings
	if !cap.VideoSupported {
		cap.Warnings = append(cap.Warnings, "视频文件上传已禁用：未配置 FFmpeg")
	}

	if !cap.AudioSupported {
		cap.Warnings = append(cap.Warnings, "音频文件上传已禁用：未配置 FFmpeg")
	}

	if cap.VideoSupported && !cap.AudioTranscription {
		cap.Warnings = append(cap.Warnings, "视频/音频可上传，但语音转文字功能不可用：未配置 RapidSpeech")
	}

	return cap
}

// GetSupportedFileTypes returns the list of supported file extensions based on capabilities.
func GetSupportedFileTypes(cap MediaCapability) map[string]string {
	supported := map[string]string{
		".pdf":      "pdf",
		".doc":      "word_legacy",
		".docx":     "word",
		".xls":      "excel_legacy",
		".xlsx":     "excel",
		".ppt":      "ppt_legacy",
		".pptx":     "ppt",
		".md":       "markdown",
		".markdown": "markdown",
		".html":     "html",
		".htm":      "html",
	}

	// Add video formats if supported
	if cap.VideoSupported {
		supported[".mp4"] = "mp4"
		supported[".avi"] = "avi"
		supported[".mkv"] = "mkv"
		supported[".mov"] = "mov"
		supported[".webm"] = "webm"
	}

	// Add audio formats if supported
	if cap.AudioSupported {
		supported[".mp3"] = "mp3"
		supported[".m4a"] = "m4a"
		supported[".wav"] = "wav"
		supported[".flac"] = "flac"
		supported[".ogg"] = "ogg"
	}

	return supported
}

// ValidateFileTypeSupport checks if a file type is supported given current capabilities.
// Returns (isSupported, warningMessage).
func ValidateFileTypeSupport(fileType string, cap MediaCapability) (bool, string) {
	videoTypes := map[string]bool{
		"mp4": true, "avi": true, "mkv": true, "mov": true, "webm": true,
	}
	audioTypes := map[string]bool{
		"mp3": true, "m4a": true, "wav": true, "flac": true, "ogg": true,
	}

	if videoTypes[fileType] {
		if !cap.VideoSupported {
			return false, "视频文件上传已禁用：系统未配置 FFmpeg。请联系管理员配置视频处理工具。"
		}
		if !cap.AudioTranscription {
			return true, "视频已上传，但语音转文字功能不可用（未配置 RapidSpeech）。视频关键帧提取正常工作。"
		}
		return true, ""
	}

	if audioTypes[fileType] {
		if !cap.AudioSupported {
			return false, "音频文件上传已禁用：系统未配置 FFmpeg。请联系管理员配置音频处理工具。"
		}
		if !cap.AudioTranscription {
			return true, "音频已上传，但语音转文字功能不可用（未配置 RapidSpeech）。文件已保存但无法提取文本内容。"
		}
		return true, ""
	}

	// Document types are always supported
	return true, ""
}

// FormatCapabilityMessage generates a user-friendly message about system capabilities.
func FormatCapabilityMessage(cap MediaCapability) string {
	if len(cap.Warnings) == 0 {
		return "系统支持所有文件类型（文档、视频、音频），所有功能正常。"
	}

	var parts []string
	parts = append(parts, "系统能力状态：")

	if cap.VideoSupported {
		parts = append(parts, "✓ 视频文件支持")
	} else {
		parts = append(parts, "✗ 视频文件不支持")
	}

	if cap.AudioSupported {
		parts = append(parts, "✓ 音频文件支持")
	} else {
		parts = append(parts, "✗ 音频文件不支持")
	}

	if cap.AudioTranscription {
		parts = append(parts, "✓ 语音转文字支持")
	} else {
		parts = append(parts, "✗ 语音转文字不支持")
	}

	if len(cap.MissingComponents) > 0 {
		parts = append(parts, "\n缺失组件："+strings.Join(cap.MissingComponents, ", "))
	}

	return strings.Join(parts, "\n")
}

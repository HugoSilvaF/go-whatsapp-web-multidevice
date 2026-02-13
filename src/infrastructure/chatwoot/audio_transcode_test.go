package chatwoot

import "testing"

func TestShouldTranscodeToMP3(t *testing.T) {
	tests := []struct {
		filePath string
		expected bool
	}{
		{filePath: "voice.ogg", expected: true},
		{filePath: "voice.opus", expected: true},
		{filePath: "voice.webm", expected: true},
		{filePath: "audio.mp3", expected: false},
		{filePath: "audio.m4a", expected: false},
		{filePath: "audio.wav", expected: false},
		{filePath: "audio.aac", expected: false},
		{filePath: "image.jpg", expected: false},
		{filePath: "document.pdf", expected: false},
	}

	for _, tt := range tests {
		got := shouldTranscodeToMP3(tt.filePath)
		if got != tt.expected {
			t.Errorf("shouldTranscodeToMP3(%q) = %v, expected %v", tt.filePath, got, tt.expected)
		}
	}
}

func TestIsAudioAttachmentByExtension(t *testing.T) {
	tests := []struct {
		filePath string
		expected bool
	}{
		{filePath: "voice.ogg", expected: true},
		{filePath: "voice.opus", expected: true},
		{filePath: "voice.mp3", expected: true},
		{filePath: "voice.wav", expected: true},
		{filePath: "photo.png", expected: false},
		{filePath: "archive.zip", expected: false},
	}

	for _, tt := range tests {
		got := isAudioAttachment(tt.filePath)
		if got != tt.expected {
			t.Errorf("isAudioAttachment(%q) = %v, expected %v", tt.filePath, got, tt.expected)
		}
	}
}

func TestShouldMarkAsRecordedAudio(t *testing.T) {
	tests := []struct {
		name     string
		filePath string
		mimeType string
		expected bool
	}{
		{
			name:     "audio mime type",
			filePath: "file.bin",
			mimeType: "audio/mpeg",
			expected: true,
		},
		{
			name:     "audio extension fallback",
			filePath: "voice.ogg",
			mimeType: "application/octet-stream",
			expected: true,
		},
		{
			name:     "non audio",
			filePath: "file.pdf",
			mimeType: "application/pdf",
			expected: false,
		},
	}

	for _, tt := range tests {
		got := shouldMarkAsRecordedAudio(tt.filePath, tt.mimeType)
		if got != tt.expected {
			t.Errorf("%s: shouldMarkAsRecordedAudio(%q, %q) = %v, expected %v", tt.name, tt.filePath, tt.mimeType, got, tt.expected)
		}
	}
}

func TestCanonicalizeMimeType(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{input: "audio/ogg; codecs=opus", expected: "audio/ogg"},
		{input: "application/ogg", expected: "audio/ogg"},
		{input: "audio/x-wav", expected: "audio/wav"},
		{input: "audio/mpeg", expected: "audio/mpeg"},
		{input: " application/octet-stream ", expected: "application/octet-stream"},
	}

	for _, tt := range tests {
		got := canonicalizeMimeType(tt.input)
		if got != tt.expected {
			t.Errorf("canonicalizeMimeType(%q) = %q, expected %q", tt.input, got, tt.expected)
		}
	}
}

func TestNormalizeAttachmentMimeType(t *testing.T) {
	tests := []struct {
		filePath string
		mimeType string
		expected string
	}{
		{
			filePath: "voice.ogg",
			mimeType: "application/ogg",
			expected: "audio/ogg",
		},
		{
			filePath: "voice.opus",
			mimeType: "application/octet-stream",
			expected: "audio/ogg",
		},
		{
			filePath: "audio.mp3",
			mimeType: "application/octet-stream",
			expected: "audio/mpeg",
		},
		{
			filePath: "image.jpg",
			mimeType: "image/jpeg",
			expected: "image/jpeg",
		},
	}

	for _, tt := range tests {
		got := normalizeAttachmentMimeType(tt.filePath, tt.mimeType)
		if got != tt.expected {
			t.Errorf("normalizeAttachmentMimeType(%q, %q) = %q, expected %q", tt.filePath, tt.mimeType, got, tt.expected)
		}
	}
}

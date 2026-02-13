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

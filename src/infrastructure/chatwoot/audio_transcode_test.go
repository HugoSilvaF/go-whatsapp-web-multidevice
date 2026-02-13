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

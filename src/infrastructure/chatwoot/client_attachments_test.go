package chatwoot

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestCreateMessageWithAttachments_SendsRecordedAudioField(t *testing.T) {
	tmpDir := t.TempDir()
	audioPath := filepath.Join(tmpDir, "voice.mp3")
	if err := os.WriteFile(audioPath, []byte("fake-mp3-data"), 0600); err != nil {
		t.Fatalf("failed to write temp audio file: %v", err)
	}

	var (
		gotRecordedAudio string
		gotAttachment    string
		gotMessageType   string
	)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseMultipartForm(32 << 20); err != nil {
			t.Fatalf("failed to parse multipart form: %v", err)
		}

		gotRecordedAudio = r.FormValue("is_recorded_audio")
		gotMessageType = r.FormValue("message_type")

		files := r.MultipartForm.File["attachments[]"]
		if len(files) != 1 {
			t.Fatalf("expected exactly 1 attachment, got %d", len(files))
		}
		gotAttachment = files[0].Filename

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":321}`))
	}))
	defer srv.Close()

	c := &Client{
		BaseURL:   srv.URL,
		APIToken:  "test-token",
		AccountID: 1,
		InboxID:   1,
		HTTPClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}

	msgID, err := c.CreateMessage(123, "audio", "incoming", []string{audioPath}, "")
	if err != nil {
		t.Fatalf("CreateMessage returned error: %v", err)
	}
	if msgID != 321 {
		t.Fatalf("expected message id 321, got %d", msgID)
	}

	if gotMessageType != "incoming" {
		t.Fatalf("expected message_type 'incoming', got %q", gotMessageType)
	}

	if gotAttachment != filepath.Base(audioPath) {
		t.Fatalf("expected attachment filename %q, got %q", filepath.Base(audioPath), gotAttachment)
	}

	var recorded []string
	if err := json.Unmarshal([]byte(gotRecordedAudio), &recorded); err != nil {
		t.Fatalf("is_recorded_audio should be a JSON array, got %q (%v)", gotRecordedAudio, err)
	}
	if len(recorded) != 1 || recorded[0] != filepath.Base(audioPath) {
		t.Fatalf("expected is_recorded_audio to contain %q, got %#v", filepath.Base(audioPath), recorded)
	}
}

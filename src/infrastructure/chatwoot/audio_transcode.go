package chatwoot

import (
	"context"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
)

var audioExtensions = map[string]struct{}{
	".aac":  {},
	".amr":  {},
	".flac": {},
	".m4a":  {},
	".mp3":  {},
	".oga":  {},
	".ogg":  {},
	".opus": {},
	".wav":  {},
	".webm": {},
}

var passthroughAudioExtensions = map[string]struct{}{
	".aac": {},
	".m4a": {},
	".mp3": {},
	".wav": {},
}

func shouldTranscodeToMP3(filePath string) bool {
	if !isAudioAttachment(filePath) {
		return false
	}
	ext := strings.ToLower(filepath.Ext(filePath))
	_, passthrough := passthroughAudioExtensions[ext]
	return !passthrough
}

func isAudioAttachment(filePath string) bool {
	ext := strings.ToLower(filepath.Ext(filePath))
	if ext != "" {
		if _, ok := audioExtensions[ext]; ok {
			return true
		}
		mimeType := mime.TypeByExtension(ext)
		if strings.HasPrefix(mimeType, "audio/") {
			return true
		}
	}

	mimeType, err := detectContentType(filePath)
	if err != nil {
		return false
	}
	return strings.HasPrefix(mimeType, "audio/")
}

func detectContentType(filePath string) (string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return "", err
	}
	defer file.Close()

	buffer := make([]byte, 512)
	n, err := file.Read(buffer)
	if err != nil && err != io.EOF {
		return "", err
	}
	if n == 0 {
		return "", nil
	}

	return http.DetectContentType(buffer[:n]), nil
}

func transcodeAudioToMP3(sourcePath string) (string, error) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		return "", fmt.Errorf("ffmpeg not found in PATH")
	}

	tmpFile, err := os.CreateTemp("", "chatwoot-audio-*.mp3")
	if err != nil {
		return "", fmt.Errorf("failed to create temp file for mp3: %w", err)
	}
	targetPath := tmpFile.Name()
	if err := tmpFile.Close(); err != nil {
		_ = os.Remove(targetPath)
		return "", fmt.Errorf("failed to close temp file: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	cmd := exec.CommandContext(
		ctx,
		"ffmpeg",
		"-y",
		"-hide_banner",
		"-loglevel", "error",
		"-i", sourcePath,
		"-vn",
		"-c:a", "libmp3lame",
		"-q:a", "4",
		targetPath,
	)
	output, err := cmd.CombinedOutput()
	if err != nil {
		_ = os.Remove(targetPath)
		if ctx.Err() == context.DeadlineExceeded {
			return "", fmt.Errorf("ffmpeg timeout while transcoding %s", sourcePath)
		}
		if len(output) > 0 {
			return "", fmt.Errorf("ffmpeg failed: %s", strings.TrimSpace(string(output)))
		}
		return "", fmt.Errorf("ffmpeg failed: %w", err)
	}

	return targetPath, nil
}

func prepareAttachmentForUpload(filePath string) (string, func()) {
	if !shouldTranscodeToMP3(filePath) {
		return filePath, func() {}
	}

	convertedPath, err := transcodeAudioToMP3(filePath)
	if err != nil {
		logrus.Warnf("Chatwoot: audio transcode failed for %s: %v. Uploading original file", filePath, err)
		return filePath, func() {}
	}

	return convertedPath, func() {
		if err := os.Remove(convertedPath); err != nil && !os.IsNotExist(err) {
			logrus.Debugf("Chatwoot: failed to cleanup temp audio file %s: %v", convertedPath, err)
		}
	}
}

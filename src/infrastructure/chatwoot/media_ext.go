package chatwoot

import "path/filepath"

func getExtensionForMediaType(mediaType, filename string) string {
	if filename != "" {
		if ext := filepath.Ext(filename); ext != "" {
			return ext
		}
	}
	switch mediaType {
	case "image":
		return ".jpg"
	case "video":
		return ".mp4"
	case "audio", "ptt":
		return ".ogg"
	case "document":
		return ".bin"
	case "sticker":
		return ".webp"
	default:
		return ""
	}
}

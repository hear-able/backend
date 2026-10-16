package speaking

import (
	"bytes"
	"net/http"
)

// detectAudio sniffs the uploaded bytes (never trusting the client's
// Content-Type) and returns a filename extension the transcription API
// accepts, or "" if the format is not supported.
func detectAudio(data []byte) string {
	switch http.DetectContentType(data) {
	case "audio/wave":
		return "wav"
	case "audio/mpeg":
		return "mp3"
	case "application/ogg", "audio/ogg":
		return "ogg"
	case "video/webm", "audio/webm":
		return "webm"
	case "video/mp4", "audio/mp4":
		return "m4a"
	}
	// MP3 without an ID3 tag starts with an MPEG frame sync.
	if len(data) >= 2 && data[0] == 0xFF && data[1]&0xE0 == 0xE0 {
		return "mp3"
	}
	// ISO BMFF (mp4/m4a) with brands net/http doesn't know, e.g. "M4A " from
	// Safari/iOS recorders.
	if len(data) >= 12 && string(data[4:8]) == "ftyp" {
		return "m4a"
	}
	// FLAC is not sniffed by net/http.
	if bytes.HasPrefix(data, []byte("fLaC")) {
		return "flac"
	}
	return ""
}

// Package audio sniffs uploaded audio so handlers never trust the client's
// Content-Type.
package audio

import (
	"bytes"
	"net/http"
)

// Format is a detected audio container.
type Format struct {
	// Ext is a filename extension the transcription API accepts.
	Ext string
	// ContentType is the MIME type to store the file with.
	ContentType string
}

var formats = map[string]Format{
	"wav":  {"wav", "audio/wav"},
	"mp3":  {"mp3", "audio/mpeg"},
	"ogg":  {"ogg", "audio/ogg"},
	"webm": {"webm", "audio/webm"},
	"m4a":  {"m4a", "audio/mp4"},
	"flac": {"flac", "audio/flac"},
}

// Detect returns the audio format of data, or ok=false if unsupported.
func Detect(data []byte) (Format, bool) {
	f, ok := formats[detectExt(data)]
	return f, ok
}

func detectExt(data []byte) string {
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

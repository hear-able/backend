package audio

import "testing"

func TestDetectAudio(t *testing.T) {
	cases := map[string][]byte{
		"webm": {0x1A, 0x45, 0xDF, 0xA3, 0x9F, 0x42, 0x86, 0x81, 0x01, 0x42, 0x82, 0x84, 'w', 'e', 'b', 'm'},
		"wav":  append([]byte("RIFF\x24\x00\x00\x00WAVEfmt "), make([]byte, 8)...),
		"ogg":  append([]byte("OggS\x00"), make([]byte, 16)...),
		"mp3":  append([]byte("ID3"), make([]byte, 16)...),
		"m4a":  append([]byte("\x00\x00\x00\x18ftypM4A \x00\x00\x00\x00"), make([]byte, 8)...),
		"":     []byte("<html>not audio</html>"),
	}
	for want, data := range cases {
		if got, _ := Detect(data); got.Ext != want {
			t.Errorf("want %q, got %q", want, got.Ext)
		}
	}
}

package syllable

import (
	"encoding/json"
	"testing"
)

func TestSplitMatchesSeed(t *testing.T) {
	cases := map[string]string{
		"Selamat Pagi":             `[{"text":"Se"},{"text":"la"},{"text":"mat","stress":true},{"break":true},{"text":"Pa"},{"text":"gi","stress":true}]`,
		"Terima Kasih":             `[{"text":"Te"},{"text":"ri"},{"text":"ma","stress":true},{"break":true},{"text":"Ka"},{"text":"sih","stress":true}]`,
		"Apa Kabar":                `[{"text":"A"},{"text":"pa","stress":true},{"break":true},{"text":"Ka"},{"text":"bar","stress":true}]`,
		"Sampai Jumpa":             `[{"text":"Sam"},{"text":"pai","stress":true},{"break":true},{"text":"Jum"},{"text":"pa","stress":true}]`,
		"Air":                      `[{"text":"A"},{"text":"ir","stress":true}]`,
		"Jangan lupa ya":           `[{"text":"Ja"},{"text":"ngan","stress":true},{"break":true},{"text":"lu"},{"text":"pa","stress":true},{"break":true},{"text":"ya","stress":true}]`,
		"Saya mau beli jeruk":      `[{"text":"Sa"},{"text":"ya","stress":true},{"break":true},{"text":"mau","stress":true},{"break":true},{"text":"be"},{"text":"li","stress":true},{"break":true},{"text":"je"},{"text":"ruk","stress":true}]`,
		"Tidak jadi, terima kasih": `[{"text":"Ti"},{"text":"dak","stress":true},{"break":true},{"text":"ja"},{"text":"di,","stress":true},{"break":true},{"text":"te"},{"text":"ri"},{"text":"ma","stress":true},{"break":true},{"text":"ka"},{"text":"sih","stress":true}]`,
		"Minta kantongnya":         `[{"text":"Min"},{"text":"ta","stress":true},{"break":true},{"text":"kan"},{"text":"tong"},{"text":"nya","stress":true}]`,
	}
	for in, want := range cases {
		got, _ := json.Marshal(Split(in))
		if string(got) != want {
			t.Errorf("%q\n got %s\nwant %s", in, got, want)
		}
	}
}

func TestSplitEdgeCases(t *testing.T) {
	if got := Split("   "); len(got) != 0 {
		t.Fatalf("blank input should give no parts, got %v", got)
	}
	got := Split("Psst!")
	if len(got) != 1 || got[0].Text != "Psst!" || !got[0].Stress {
		t.Fatalf("word without vowels should stay whole, got %+v", got)
	}
}

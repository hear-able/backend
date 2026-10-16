// Package syllable splits Indonesian sentences into the syllable guide the
// speaking practice screen renders. It is rule based, so prefixes such as
// "ber-" in "berapa" are split phonetically (be-ra-pa), which is fine for a
// rhythm guide.
package syllable

import (
	"strings"
	"unicode"
)

// Part is one item of the guide: either a syllable or a word break. The JSON
// shape matches sentences.syllables in the seed data.
type Part struct {
	Text   string `json:"text,omitempty"`
	Stress bool   `json:"stress,omitempty"`
	Break  bool   `json:"break,omitempty"`
}

// Split returns the syllable guide for a sentence. The last syllable of each
// word is stressed, matching the seed content.
func Split(sentence string) []Part {
	parts := []Part{}
	for _, word := range strings.Fields(sentence) {
		syllables := splitWord(word)
		if len(syllables) == 0 {
			continue
		}
		if len(parts) > 0 {
			parts = append(parts, Part{Break: true})
		}
		for i, s := range syllables {
			parts = append(parts, Part{Text: s, Stress: i == len(syllables)-1})
		}
	}
	return parts
}

var digraphs = []string{"ng", "ny", "kh", "sy"}

var finalDiphthongs = map[string]bool{"ai": true, "au": true, "oi": true, "ei": true}

type unit struct {
	text  string // original letters (case preserved) plus trailing punctuation
	lower string
	vowel bool
}

func isVowel(r rune) bool { return strings.ContainsRune("aeiou", unicode.ToLower(r)) }

// units tokenises a word into letters and consonant digraphs. Non-letters
// are attached to the neighbouring unit so "jadi," keeps its comma.
func units(word string) []unit {
	runes := []rune(word)
	var out []unit
	var prefix string
	for i := 0; i < len(runes); i++ {
		r := runes[i]
		if !unicode.IsLetter(r) {
			if len(out) == 0 {
				prefix += string(r)
			} else {
				out[len(out)-1].text += string(r)
			}
			continue
		}
		u := unit{text: string(r), lower: strings.ToLower(string(r)), vowel: isVowel(r)}
		if !u.vowel && i+1 < len(runes) {
			pair := strings.ToLower(string(runes[i : i+2]))
			for _, d := range digraphs {
				if pair == d {
					u = unit{text: string(runes[i : i+2]), lower: pair}
					i++
					break
				}
			}
		}
		if prefix != "" {
			u.text = prefix + u.text
			prefix = ""
		}
		out = append(out, u)
	}
	if prefix != "" && len(out) > 0 {
		out[len(out)-1].text += prefix
	}
	return out
}

func splitWord(word string) []string {
	us := units(word)
	if len(us) == 0 {
		if strings.TrimSpace(word) == "" {
			return nil
		}
		return []string{word}
	}

	// Nucleus positions; a final diphthong (pantai, pulau) is one nucleus.
	var nuclei [][2]int // [start, end) unit indices
	for i := 0; i < len(us); i++ {
		if !us[i].vowel {
			continue
		}
		end := i + 1
		if i+2 == len(us) && us[i+1].vowel && finalDiphthongs[us[i].lower+us[i+1].lower] {
			end = i + 2
		}
		nuclei = append(nuclei, [2]int{i, end})
		i = end - 1
	}
	if len(nuclei) == 0 {
		return []string{join(us)}
	}

	// Cut points between consecutive nuclei: no consonant -> split between
	// vowels; one consonant -> it starts the next syllable; two or more ->
	// the first closes the previous syllable.
	cuts := []int{0}
	for k := 0; k+1 < len(nuclei); k++ {
		gap := nuclei[k+1][0] - nuclei[k][1]
		cut := nuclei[k][1]
		if gap >= 2 {
			cut++
		} else if gap == 1 {
			cut = nuclei[k+1][0] - 1
		}
		cuts = append(cuts, cut)
	}
	cuts = append(cuts, len(us))

	out := make([]string, 0, len(nuclei))
	for i := 0; i+1 < len(cuts); i++ {
		out = append(out, join(us[cuts[i]:cuts[i+1]]))
	}
	return out
}

func join(us []unit) string {
	var b strings.Builder
	for _, u := range us {
		b.WriteString(u.text)
	}
	return b.String()
}

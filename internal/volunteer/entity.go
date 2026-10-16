package volunteer

import "hearable/backend/package/audio"

const (
	ModuleTypeListening = "listening"
	ModuleTypeSpeaking  = "speaking"

	QuestionTypeChoice = "choice"
	QuestionTypeInput  = "input"

	// MaxAudioBytes bounds each uploaded question audio.
	MaxAudioBytes = 10 << 20
	// MaxItems bounds questions or sentences per module.
	MaxItems = 10
	// MaxListeningBodyBytes bounds the whole multipart module submit.
	MaxListeningBodyBytes = MaxItems*MaxAudioBytes + (1 << 20)

	minOptions = 2
	maxOptions = 4

	maxTitleRunes         = 120
	maxLabelRunes         = 60
	maxDescriptionRunes   = 1000
	maxPromptRunes        = 300
	maxTranscriptionRunes = 2000
	maxOptionRunes        = 300
	maxAnswerRunes        = 2000
	maxSentenceRunes      = 300
)

// TranscriptionResponse is returned before anything is stored, so the
// volunteer can correct both fields.
type TranscriptionResponse struct {
	Transcription string `json:"transcription"`
	Prompt        string `json:"prompt"`
}

type OptionInput struct {
	Text      string `json:"text"`
	IsCorrect bool   `json:"is_correct"`
}

type ListeningQuestionInput struct {
	Prompt            string        `json:"prompt"`
	Transcription     string        `json:"transcription"`
	Type              string        `json:"type"`
	Options           []OptionInput `json:"options"`
	AnswerExplanation string        `json:"answer_explanation"`
	ExpectedAnswer    string        `json:"expected_answer"`
}

// CreateListeningRequest is the JSON "payload" field of the multipart
// submit. Question i's audio is the file field "audio_i".
type CreateListeningRequest struct {
	Title       string                   `json:"title"`
	Label       string                   `json:"label"`
	Description string                   `json:"description"`
	Questions   []ListeningQuestionInput `json:"questions"`
}

type SentenceInput struct {
	Text        string `json:"text"`
	Description string `json:"description"`
}

type CreateSpeakingRequest struct {
	Title       string          `json:"title"`
	Label       string          `json:"label"`
	Description string          `json:"description"`
	Sentences   []SentenceInput `json:"sentences"`
}

// AudioFile is an uploaded file whose format has already been sniffed.
type AudioFile struct {
	Data   []byte
	Format audio.Format
}

// CreatedModule is returned after submit. DifficultyExplanation comes from
// the AI assessment and is not persisted.
type CreatedModule struct {
	ID                    string `json:"id"`
	Type                  string `json:"type"`
	Label                 string `json:"label"`
	Title                 string `json:"title"`
	Description           string `json:"description"`
	Difficulty            int    `json:"difficulty"`
	DifficultyLabel       string `json:"difficulty_label"`
	DifficultyExplanation string `json:"difficulty_explanation"`
	EstimatedMinutes      int    `json:"estimated_minutes"`
	ItemCount             int    `json:"item_count"`
}

type moduleRecord struct {
	Label            string
	Title            string
	Description      string
	Difficulty       int
	EstimatedMinutes int
	CreatedBy        string
}

type optionRecord struct {
	Text      string
	IsCorrect bool
}

type listeningQuestionRecord struct {
	Type              string
	Prompt            string
	Transcription     string
	AudioURL          string
	Options           []optionRecord
	AnswerExplanation *string
	ExpectedAnswer    string
}

type sentenceRecord struct {
	Text        string
	Description *string
	Syllables   []byte
}

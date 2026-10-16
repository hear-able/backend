package volunteer

import (
	"context"
	"database/sql"
	"fmt"
)

type Repository struct {
	db *sql.DB
}

func NewRepository(db *sql.DB) *Repository {
	return &Repository{db: db}
}

// CreateListeningModule inserts the module, its questions, answers and audio
// rows in one transaction.
func (r *Repository) CreateListeningModule(ctx context.Context, m moduleRecord, questions []listeningQuestionRecord) (string, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return "", fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	var moduleID string
	if err := tx.QueryRowContext(ctx, queryInsertListeningModule,
		m.Label, m.Title, m.Description, m.Difficulty, m.EstimatedMinutes, m.CreatedBy).Scan(&moduleID); err != nil {
		return "", fmt.Errorf("insert listening module: %w", err)
	}

	for pos, q := range questions {
		var audioID, questionID string
		if err := tx.QueryRowContext(ctx, queryInsertAudio, q.Transcription, q.AudioURL).Scan(&audioID); err != nil {
			return "", fmt.Errorf("insert audio: %w", err)
		}
		if err := tx.QueryRowContext(ctx, queryInsertQuestion, q.Type, q.Prompt, audioID).Scan(&questionID); err != nil {
			return "", fmt.Errorf("insert question: %w", err)
		}

		switch q.Type {
		case QuestionTypeChoice:
			if _, err := tx.ExecContext(ctx, queryInsertChoiceQuestion, questionID, q.AnswerExplanation); err != nil {
				return "", fmt.Errorf("insert choice question: %w", err)
			}
			for i, o := range q.Options {
				if _, err := tx.ExecContext(ctx, queryInsertChoiceOption, questionID, o.Text, o.IsCorrect, i); err != nil {
					return "", fmt.Errorf("insert choice option: %w", err)
				}
			}
		case QuestionTypeInput:
			if _, err := tx.ExecContext(ctx, queryInsertInputQuestion, questionID, q.ExpectedAnswer); err != nil {
				return "", fmt.Errorf("insert input question: %w", err)
			}
		default:
			return "", fmt.Errorf("unknown question type %q", q.Type)
		}

		if _, err := tx.ExecContext(ctx, queryInsertListeningModuleQuestion, moduleID, questionID, pos); err != nil {
			return "", fmt.Errorf("link module question: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return "", fmt.Errorf("commit listening module: %w", err)
	}
	return moduleID, nil
}

// CreateSpeakingModule inserts the module and its sentences in one
// transaction.
func (r *Repository) CreateSpeakingModule(ctx context.Context, m moduleRecord, sentences []sentenceRecord) (string, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return "", fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	var moduleID string
	if err := tx.QueryRowContext(ctx, queryInsertSpeakingModule,
		m.Label, m.Title, m.Description, m.Difficulty, m.EstimatedMinutes, m.CreatedBy).Scan(&moduleID); err != nil {
		return "", fmt.Errorf("insert speaking module: %w", err)
	}

	for pos, s := range sentences {
		var sentenceID string
		if err := tx.QueryRowContext(ctx, queryInsertSentence, s.Text, s.Description, string(s.Syllables)).Scan(&sentenceID); err != nil {
			return "", fmt.Errorf("insert sentence: %w", err)
		}
		if _, err := tx.ExecContext(ctx, queryInsertSpeakingModuleSentence, moduleID, sentenceID, pos); err != nil {
			return "", fmt.Errorf("link module sentence: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return "", fmt.Errorf("commit speaking module: %w", err)
	}
	return moduleID, nil
}

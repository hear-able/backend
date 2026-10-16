package volunteer

const (
	queryInsertAudio = `
		INSERT INTO audios (description, url)
		VALUES ($1, $2)
		RETURNING id`

	queryInsertQuestion = `
		INSERT INTO questions (type, prompt, audio_id)
		VALUES ($1, $2, $3)
		RETURNING id`

	queryInsertChoiceQuestion = `
		INSERT INTO choice_questions (question_id, answer_explanation)
		VALUES ($1, $2)`

	queryInsertChoiceOption = `
		INSERT INTO choice_options (question_id, option_text, is_correct, position)
		VALUES ($1, $2, $3, $4)`

	queryInsertInputQuestion = `
		INSERT INTO input_questions (question_id, expected_answer)
		VALUES ($1, $2)`

	// New modules go after existing ones at the same difficulty ordering.
	queryInsertListeningModule = `
		INSERT INTO listening_modules (label, title, description, difficulty, estimated_minutes, position, created_by)
		SELECT $1, $2, $3, $4, $5, COALESCE(MAX(position), -1) + 1, $6
		FROM listening_modules
		RETURNING id`

	queryInsertListeningModuleQuestion = `
		INSERT INTO listening_module_questions (module_id, question_id, position)
		VALUES ($1, $2, $3)`

	queryInsertSentence = `
		INSERT INTO sentences (text, description, syllables)
		VALUES ($1, $2, $3)
		RETURNING id`

	queryInsertSpeakingModule = `
		INSERT INTO speaking_modules (label, title, description, difficulty, estimated_minutes, position, created_by)
		SELECT $1, $2, $3, $4, $5, COALESCE(MAX(position), -1) + 1, $6
		FROM speaking_modules
		RETURNING id`

	queryInsertSpeakingModuleSentence = `
		INSERT INTO speaking_module_sentences (module_id, sentence_id, position)
		VALUES ($1, $2, $3)`
)

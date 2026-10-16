package listening

const (
	queryListModules = `
		SELECT m.id, m.label, m.title, m.description, m.difficulty, m.estimated_minutes,
		       count(lmq.question_id)
		FROM listening_modules m
		LEFT JOIN listening_module_questions lmq ON lmq.module_id = m.id
		WHERE ($1::smallint IS NULL OR m.difficulty = $1)
		GROUP BY m.id
		ORDER BY m.difficulty, m.position, m.title`

	queryGetModule = `
		SELECT m.id, m.label, m.title, m.description, m.difficulty, m.estimated_minutes,
		       (SELECT count(*) FROM listening_module_questions WHERE module_id = m.id)
		FROM listening_modules m
		WHERE m.id = $1`

	queryListQuestions = `
		SELECT q.id, q.type, q.prompt, lmq.position, a.url, a.duration_ms, a.hint_text
		FROM listening_module_questions lmq
		JOIN questions q ON q.id = lmq.question_id
		LEFT JOIN audios a ON a.id = q.audio_id
		WHERE lmq.module_id = $1
		ORDER BY lmq.position`

	queryListModuleOptions = `
		SELECT co.id, co.question_id, co.option_text
		FROM choice_options co
		JOIN listening_module_questions lmq ON lmq.question_id = co.question_id
		WHERE lmq.module_id = $1
		ORDER BY co.question_id, co.position`

	queryGetQuestionKey = `
		SELECT q.id, q.type, q.prompt,
		       COALESCE(a.description, ''),
		       COALESCE(iq.expected_answer, ''),
		       COALESCE(cq.answer_explanation, '')
		FROM listening_module_questions lmq
		JOIN questions q ON q.id = lmq.question_id
		LEFT JOIN audios a ON a.id = q.audio_id
		LEFT JOIN input_questions iq ON iq.question_id = q.id
		LEFT JOIN choice_questions cq ON cq.question_id = q.id
		WHERE lmq.module_id = $1 AND q.id = $2`

	queryGetQuestionOptions = `
		SELECT id, is_correct FROM choice_options WHERE question_id = $1`

	queryInsertSession = `
		INSERT INTO listening_sessions (user_id, module_id)
		VALUES ($1, $2)
		RETURNING id`

	queryGetSession = `
		SELECT s.id, s.user_id, s.module_id, m.title, m.difficulty, s.status, s.started_at, s.completed_at
		FROM listening_sessions s
		JOIN listening_modules m ON m.id = s.module_id
		WHERE s.id = $1`

	queryListAnswers = `
		SELECT a.question_id, a.status, a.selected_option_id, a.answer_text, a.is_correct, a.score,
		       a.feedback, a.answered_at,
		       CASE WHEN a.status = 'answered' AND q.type = 'choice' THEN
		           (SELECT co.id FROM choice_options co WHERE co.question_id = a.question_id AND co.is_correct LIMIT 1)
		       END
		FROM listening_session_answers a
		JOIN questions q ON q.id = a.question_id
		WHERE a.session_id = $1
		ORDER BY a.answered_at`

	// queryClaimAnswer inserts the answer, or replaces a previous failed (or
	// stale in-flight) evaluation. No row returned means a final answer
	// already exists or another request is evaluating it right now.
	queryClaimAnswer = `
		INSERT INTO listening_session_answers
		    (session_id, question_id, status, selected_option_id, answer_text, is_correct, score, feedback, ai_model, answered_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, now())
		ON CONFLICT (session_id, question_id) DO UPDATE SET
		    status = EXCLUDED.status,
		    selected_option_id = EXCLUDED.selected_option_id,
		    answer_text = EXCLUDED.answer_text,
		    is_correct = EXCLUDED.is_correct,
		    score = EXCLUDED.score,
		    feedback = EXCLUDED.feedback,
		    ai_model = EXCLUDED.ai_model,
		    answered_at = now()
		WHERE listening_session_answers.status = 'evaluation_failed'
		   OR (listening_session_answers.status = 'evaluating'
		       AND listening_session_answers.answered_at < now() - interval '2 minutes')
		RETURNING answered_at`

	queryFinalizeAnswer = `
		UPDATE listening_session_answers
		SET status = $3, is_correct = $4, score = $5, feedback = $6, ai_model = $7, answered_at = now()
		WHERE session_id = $1 AND question_id = $2 AND status = 'evaluating'
		RETURNING answered_at`

	queryCompleteSession = `
		UPDATE listening_sessions
		SET status = 'completed', completed_at = now()
		WHERE id = $1 AND status = 'in_progress'`

	querySummary = `
		SELECT
		    (SELECT count(*) FROM listening_module_questions WHERE module_id = $2),
		    count(*) FILTER (WHERE status = 'answered'),
		    count(*) FILTER (WHERE status = 'skipped'),
		    count(*) FILTER (WHERE is_correct)
		FROM listening_session_answers
		WHERE session_id = $1`
)

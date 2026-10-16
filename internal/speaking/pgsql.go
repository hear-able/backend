package speaking

const (
	queryListModules = `
		SELECT m.id, m.label, m.title, m.description, m.difficulty, m.estimated_minutes,
		       count(sms.sentence_id)
		FROM speaking_modules m
		LEFT JOIN speaking_module_sentences sms ON sms.module_id = m.id
		WHERE ($1::smallint IS NULL OR m.difficulty = $1)
		GROUP BY m.id
		ORDER BY m.difficulty, m.position, m.title`

	queryGetModule = `
		SELECT m.id, m.label, m.title, m.description, m.difficulty, m.estimated_minutes,
		       (SELECT count(*) FROM speaking_module_sentences WHERE module_id = m.id)
		FROM speaking_modules m
		WHERE m.id = $1`

	queryListSentences = `
		SELECT s.id, s.text, s.syllables, s.example_audio_url, sms.position
		FROM speaking_module_sentences sms
		JOIN sentences s ON s.id = sms.sentence_id
		WHERE sms.module_id = $1
		ORDER BY sms.position`

	queryGetSentenceKey = `
		SELECT s.id, s.text, sms.position
		FROM speaking_module_sentences sms
		JOIN sentences s ON s.id = sms.sentence_id
		WHERE sms.module_id = $1 AND s.id = $2`

	queryInsertSession = `
		INSERT INTO speaking_sessions (user_id, module_id)
		VALUES ($1, $2)
		RETURNING id`

	queryGetSession = `
		SELECT s.id, s.user_id, s.module_id, m.title, m.difficulty, s.status, s.started_at, s.completed_at
		FROM speaking_sessions s
		JOIN speaking_modules m ON m.id = s.module_id
		WHERE s.id = $1`

	queryListAttempts = `
		SELECT sentence_id, status, transcript, score, explanation, feedback, created_at
		FROM speaking_session_comments
		WHERE session_id = $1 AND status <> 'evaluating'
		ORDER BY position`

	// queryClaimAttempt reserves the (session, sentence) slot before the AI
	// is called so concurrent uploads cannot both be charged. A stale
	// in-flight claim (crashed request) can be taken over after 2 minutes.
	queryClaimAttempt = `
		INSERT INTO speaking_session_comments
		    (session_id, sentence_id, position, status, transcript, score, explanation, feedback, ai_model, created_at)
		VALUES ($1, $2, $3, $4, NULL, NULL, NULL, NULL, NULL, now())
		ON CONFLICT (session_id, sentence_id) DO UPDATE SET
		    status = EXCLUDED.status,
		    created_at = now()
		WHERE speaking_session_comments.status = 'evaluating'
		  AND speaking_session_comments.created_at < now() - interval '2 minutes'
		RETURNING created_at`

	queryFinalizeAttempt = `
		UPDATE speaking_session_comments
		SET status = $3, transcript = $4, score = $5, explanation = $6, feedback = $7, ai_model = $8, created_at = now()
		WHERE session_id = $1 AND sentence_id = $2 AND status = 'evaluating'
		RETURNING created_at`

	queryReleaseAttempt = `
		DELETE FROM speaking_session_comments
		WHERE session_id = $1 AND sentence_id = $2 AND status = 'evaluating'`

	queryCompleteSession = `
		UPDATE speaking_sessions
		SET status = 'completed', completed_at = now()
		WHERE id = $1 AND status = 'in_progress'`

	querySummary = `
		SELECT
		    (SELECT count(*) FROM speaking_module_sentences WHERE module_id = $2),
		    count(*) FILTER (WHERE status = 'evaluated'),
		    count(*) FILTER (WHERE status = 'skipped'),
		    round(avg(score) FILTER (WHERE status = 'evaluated'))::int
		FROM speaking_session_comments
		WHERE session_id = $1`
)

package progress

const (
	queryCompletionsBetween = `
		SELECT 'listening', completed_at FROM listening_sessions
		WHERE user_id = $1 AND completed_at >= $2 AND completed_at < $3
		UNION ALL
		SELECT 'speaking', completed_at FROM speaking_sessions
		WHERE user_id = $1 AND completed_at >= $2 AND completed_at < $3`

	// queryActiveDates returns distinct local dates with a completed session,
	// newest first, used for the streak.
	queryActiveDates = `
		SELECT DISTINCT (completed_at AT TIME ZONE $2)::date AS d
		FROM (
		    SELECT completed_at FROM listening_sessions WHERE user_id = $1 AND completed_at IS NOT NULL
		    UNION ALL
		    SELECT completed_at FROM speaking_sessions WHERE user_id = $1 AND completed_at IS NOT NULL
		) c
		ORDER BY d DESC
		LIMIT 366`

	queryHistory = `
		SELECT * FROM (
		    SELECT 'listening' AS type, s.id, s.module_id, m.title, m.difficulty, s.status, s.started_at, s.completed_at,
		           (SELECT count(*) FROM listening_module_questions WHERE module_id = s.module_id) AS total,
		           (SELECT count(*) FROM listening_session_answers a
		             WHERE a.session_id = s.id AND a.status IN ('answered', 'skipped')) AS done,
		           (SELECT count(*) FROM listening_session_answers a
		             WHERE a.session_id = s.id AND a.is_correct)::int AS correct,
		           NULL::int AS average_score
		    FROM listening_sessions s
		    JOIN listening_modules m ON m.id = s.module_id
		    WHERE s.user_id = $1
		    UNION ALL
		    SELECT 'speaking', s.id, s.module_id, m.title, m.difficulty, s.status, s.started_at, s.completed_at,
		           (SELECT count(*) FROM speaking_module_sentences WHERE module_id = s.module_id),
		           (SELECT count(*) FROM speaking_session_comments c
		             WHERE c.session_id = s.id AND c.status IN ('evaluated', 'skipped')),
		           NULL::int,
		           (SELECT round(avg(c.score))::int FROM speaking_session_comments c
		             WHERE c.session_id = s.id AND c.status = 'evaluated')
		    FROM speaking_sessions s
		    JOIN speaking_modules m ON m.id = s.module_id
		    WHERE s.user_id = $1
		) h
		WHERE ($2::text IS NULL OR h.status = $2)
		  AND ($3::timestamptz IS NULL OR (h.started_at, h.id) < ($3, $4::uuid))
		ORDER BY h.started_at DESC, h.id DESC
		LIMIT $5`
)

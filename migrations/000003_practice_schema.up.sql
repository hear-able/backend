-- Listening content -----------------------------------------------------------

CREATE TABLE audios (
    id UUID DEFAULT uuid_generate_v4() PRIMARY KEY,
    description TEXT,
    url VARCHAR(1000) NOT NULL,
    hint_text TEXT,
    duration_ms INTEGER CHECK (duration_ms IS NULL OR duration_ms > 0)
);

CREATE TABLE listening_modules (
    id UUID DEFAULT uuid_generate_v4() PRIMARY KEY,
    label VARCHAR(255) NOT NULL,
    title VARCHAR(255) NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    difficulty SMALLINT NOT NULL CHECK (difficulty BETWEEN 1 AND 4),
    estimated_minutes SMALLINT NOT NULL DEFAULT 5 CHECK (estimated_minutes > 0),
    position INTEGER NOT NULL DEFAULT 0
);

CREATE INDEX listening_modules_difficulty_idx ON listening_modules (difficulty, position);

CREATE TABLE questions (
    id UUID DEFAULT uuid_generate_v4() PRIMARY KEY,
    type VARCHAR(10) NOT NULL CHECK (type IN ('choice', 'input')),
    prompt TEXT NOT NULL,
    audio_id UUID REFERENCES audios (id),
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE choice_questions (
    question_id UUID PRIMARY KEY REFERENCES questions (id) ON DELETE CASCADE,
    answer_explanation TEXT
);

CREATE TABLE input_questions (
    question_id UUID PRIMARY KEY REFERENCES questions (id) ON DELETE CASCADE,
    expected_answer TEXT NOT NULL
);

CREATE TABLE choice_options (
    id UUID DEFAULT uuid_generate_v4() PRIMARY KEY,
    question_id UUID NOT NULL REFERENCES choice_questions (question_id) ON DELETE CASCADE,
    option_text TEXT NOT NULL,
    is_correct BOOLEAN NOT NULL DEFAULT FALSE,
    position INTEGER NOT NULL,
    UNIQUE (question_id, position)
);

CREATE TABLE listening_module_questions (
    module_id UUID NOT NULL REFERENCES listening_modules (id) ON DELETE CASCADE,
    question_id UUID NOT NULL REFERENCES questions (id) ON DELETE CASCADE,
    position INTEGER NOT NULL,
    UNIQUE (module_id, question_id),
    UNIQUE (module_id, position)
);

-- Listening sessions ----------------------------------------------------------

CREATE TABLE listening_sessions (
    id UUID DEFAULT uuid_generate_v4() PRIMARY KEY,
    user_id UUID NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    module_id UUID NOT NULL REFERENCES listening_modules (id),
    status VARCHAR(20) NOT NULL DEFAULT 'in_progress' CHECK (status IN ('in_progress', 'completed')),
    started_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP,
    completed_at TIMESTAMP WITH TIME ZONE
);

CREATE INDEX listening_sessions_user_started_idx ON listening_sessions (user_id, started_at DESC, id DESC);
CREATE INDEX listening_sessions_user_completed_idx ON listening_sessions (user_id, completed_at);

CREATE TABLE listening_session_answers (
    id UUID DEFAULT uuid_generate_v4() PRIMARY KEY,
    session_id UUID NOT NULL REFERENCES listening_sessions (id) ON DELETE CASCADE,
    question_id UUID NOT NULL REFERENCES questions (id),
    status VARCHAR(20) NOT NULL CHECK (status IN ('evaluating', 'answered', 'skipped', 'evaluation_failed')),
    selected_option_id UUID REFERENCES choice_options (id),
    answer_text TEXT,
    is_correct BOOLEAN,
    score SMALLINT CHECK (score IS NULL OR score BETWEEN 0 AND 100),
    feedback JSONB,
    ai_model VARCHAR(100),
    answered_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE (session_id, question_id)
);

-- Speaking content ------------------------------------------------------------

CREATE TABLE sentences (
    id UUID DEFAULT uuid_generate_v4() PRIMARY KEY,
    text TEXT NOT NULL,
    description TEXT,
    syllables JSONB NOT NULL DEFAULT '[]'::jsonb,
    example_audio_url VARCHAR(1000)
);

CREATE TABLE speaking_modules (
    id UUID DEFAULT uuid_generate_v4() PRIMARY KEY,
    label VARCHAR(255) NOT NULL,
    title VARCHAR(255) NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    difficulty SMALLINT NOT NULL CHECK (difficulty BETWEEN 1 AND 4),
    estimated_minutes SMALLINT NOT NULL DEFAULT 5 CHECK (estimated_minutes > 0),
    position INTEGER NOT NULL DEFAULT 0
);

CREATE INDEX speaking_modules_difficulty_idx ON speaking_modules (difficulty, position);

CREATE TABLE speaking_module_sentences (
    id UUID DEFAULT uuid_generate_v4() PRIMARY KEY,
    module_id UUID NOT NULL REFERENCES speaking_modules (id) ON DELETE CASCADE,
    sentence_id UUID NOT NULL REFERENCES sentences (id) ON DELETE CASCADE,
    position INTEGER NOT NULL,
    UNIQUE (module_id, sentence_id),
    UNIQUE (module_id, position)
);

-- Speaking sessions -----------------------------------------------------------

CREATE TABLE speaking_sessions (
    id UUID DEFAULT uuid_generate_v4() PRIMARY KEY,
    user_id UUID NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    module_id UUID NOT NULL REFERENCES speaking_modules (id),
    status VARCHAR(20) NOT NULL DEFAULT 'in_progress' CHECK (status IN ('in_progress', 'completed')),
    started_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP,
    completed_at TIMESTAMP WITH TIME ZONE
);

CREATE INDEX speaking_sessions_user_started_idx ON speaking_sessions (user_id, started_at DESC, id DESC);
CREATE INDEX speaking_sessions_user_completed_idx ON speaking_sessions (user_id, completed_at);

CREATE TABLE speaking_session_comments (
    id UUID DEFAULT uuid_generate_v4() PRIMARY KEY,
    session_id UUID NOT NULL REFERENCES speaking_sessions (id) ON DELETE CASCADE,
    sentence_id UUID NOT NULL REFERENCES sentences (id),
    position INTEGER NOT NULL,
    status VARCHAR(20) NOT NULL CHECK (status IN ('evaluating', 'evaluated', 'skipped')),
    transcript TEXT,
    score SMALLINT CHECK (score IS NULL OR score BETWEEN 0 AND 100),
    explanation TEXT,
    feedback JSONB,
    ai_model VARCHAR(100),
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE (session_id, sentence_id)
);

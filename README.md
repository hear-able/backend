# Hear-able backend

Go API for Hear-able, a listening and speaking practice app for hearing-aid users.

- API: `https://api.hearable.nighttealabs.tech`
- Docs (Swagger UI): `https://docs.hearable.nighttealabs.tech`, built from `docs/openapi.yaml`

## Layout

```
cmd/main.go            entrypoint: config → database → router
config/                config loader (env.yaml, overridden by environment variables)
database/              Postgres connection (pgx via database/sql)
route/                 router, middleware wiring, graceful shutdown
internal/auth          register / login / refresh / logout / me
internal/listening     listening modules, sessions, answers (multiple choice + AI-graded input)
internal/speaking      speaking modules, sessions, recorded attempts (transcription + AI feedback)
internal/progress      weekly activity tracker and practice history
internal/volunteer     volunteer content: audio transcription, module submit with AI difficulty
internal/ai            OpenAI client (chat completions with JSON schema + audio transcription)
package/               shared helpers: errors, jsonresponder, middleware, request, token, difficulty,
                       audio (format sniffing), storage (Supabase), syllable (Indonesian splitter)
migrations/            golang-migrate SQL migrations (000004 seeds demo content)
```

Each feature package follows `handler → service → repo`. SQL lives in `pgsql.go`.

## Configuration

Values are read from `env.yaml` (path overridable with `CONFIG_PATH`). Environment variables take
precedence. See `env.yaml.example`.

| Env var | Required | Default | Notes |
|---|---|---|---|
| `DATABASE_URL` | yes | | Postgres connection string |
| `JWT_SECRET` | yes | | at least 32 characters, used to sign access tokens |
| `OPENAI_API_KEY` | yes | | backend only, never sent to the frontend |
| `OPENAI_EVAL_MODEL` | no | `gpt-4o-mini` | must support `response_format: json_schema` |
| `OPENAI_TRANSCRIBE_MODEL` | no | `gpt-4o-mini-transcribe` | |
| `OPENAI_BASE_URL` | no | `https://api.openai.com/v1` | |
| `APP_TIMEZONE` | no | `Asia/Jakarta` | timezone for activity days and weeks |
| `SUPABASE_URL` | yes | | Supabase project URL, used for volunteer audio uploads |
| `SUPABASE_SERVICE_KEY` | yes | | service role key, backend only |
| `SUPABASE_BUCKET` | no | `audio` | must be a public bucket |
| `CORS_ORIGINS` | no | docs domain + `http://localhost:3000` + `http://localhost:5173` | comma-separated. Add the web app's production origin. |
| `PORT` | no | `8080` | |

Production (`.github/workflows/deployment.yaml`) builds `DATABASE_URL` from the existing `DB_*`
secrets. It also needs the repository secrets **`JWT_SECRET`**, **`OPENAI_API_KEY`**,
**`SUPABASE_URL`** and **`SUPABASE_SERVICE_KEY`**.

## Running locally

```sh
cp env.yaml.example env.yaml   # fill in values
make migrate-up                # needs golang-migrate and yq
go run ./cmd
go test ./...
```

Create a migration: `make migrate-create name=<migration_name>`

## Content and audio files

`000004_seed_content` adds 16 Indonesian demo modules: 2 listening and 2 speaking modules for each
of the 4 levels.

`000006_listening_seed_v2` replaces the listening content with recorded clips in the public Supabase
Storage bucket `audio`. Each file is named `L<level>m<module>q<question>.m4a`, for example
`L1m1q1.m4a` to `L4m2q4.m4a` (32 clips):

```
https://vviccuyvlvltctausofu.supabase.co/storage/v1/object/public/audio/L1m1q1.m4a
```

`audios.description` holds the exact transcript of each clip and `duration_ms` its real length.

Speaking sentences have no audio: the mobile app reads `text` aloud with on-device text-to-speech
(`000005_drop_sentence_audio` removes `sentences.example_audio_url`).

## Scoring

All scores are practice feedback, **not** a clinical or audiological assessment.

- **Listening, multiple choice:** graded exactly against `choice_options.is_correct`. The score is 100 or 0, with no AI.
- **Listening, input:** the AI compares the answer with `input_questions.expected_answer`, which is
  loaded from the database and never sent by the client. Paraphrases are accepted.
  - The score is 0–100, with criteria `main_idea`, `details` and `relevance`.
  - `score >= 70` counts as correct.
- **Speaking:**
  - The recording is transcribed by OpenAI and then **discarded**. It is never stored.
  - The AI compares the transcript with `sentences.text` and gives a 0–100 score with criteria
    `kejelasan` (how recognisable the words were) and `kelengkapan` (how complete the phrase was).
  - The model only sees text, so this measures intelligibility to speech recognition, **not**
    pronunciation.
  - Accents and speech differences are not treated as errors.

## AI reliability

- Each call has a 30 s timeout. 429, 5xx and network errors are retried once, never in a loop.
- Model output must match a strict JSON schema, and scores must be 0–100. Otherwise it is rejected.
- An item slot is claimed with status `evaluating` before the AI is called. A double submit
  returns 409 instead of paying for a second evaluation.
- If listening input grading fails, the answer is kept as `evaluation_failed` and the API returns
  503/502 with `retryable: true`. Send the same request again to retry.
- If speaking fails, nothing is stored and the user records again.
- Logs include the model, attempt number and latency. They never include audio, answers or keys.
- Answers and transcripts are passed to the model as JSON data, and the model is told to ignore
  any instructions inside them.

## Volunteer content

Accounts registered with `role: volunteer` (the web app does this) can create modules under
`/v1/volunteer/*`:

1. `POST /listening/transcriptions` takes one audio file and returns its transcription plus an
   AI-drafted instruction. Nothing is stored at this point.
2. `POST /listening/modules` takes a multipart form: a `payload` JSON field and `audio_0..n`
   files. The input is validated, AI assesses difficulty (1–4), the audio files are uploaded to
   Supabase, and everything is inserted in one transaction. If the insert fails, the uploads are
   deleted.
3. `POST /speaking/modules` takes JSON. Syllables are generated by `package/syllable`, and AI
   assesses difficulty.

The transcription is saved to `audios.description` and the instruction to `questions.prompt`.
`created_by` on the module tables records the volunteer.

## Not implemented yet

- Forgot password and email verification (needs an email provider)
- Editing or deleting volunteer modules, and reviewing content before it is published
  (volunteer modules go live as soon as they are submitted)
- Profile editing

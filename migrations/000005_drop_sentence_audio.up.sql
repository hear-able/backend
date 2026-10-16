-- Speaking sentences are read aloud by the mobile app (text-to-speech), so
-- they no longer carry an example audio file.
ALTER TABLE sentences DROP COLUMN example_audio_url;

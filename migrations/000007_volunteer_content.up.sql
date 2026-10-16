-- Modules contributed by volunteers from the web app. Seed modules keep NULL.
ALTER TABLE listening_modules
    ADD COLUMN created_by UUID REFERENCES users (id) ON DELETE SET NULL;

ALTER TABLE speaking_modules
    ADD COLUMN created_by UUID REFERENCES users (id) ON DELETE SET NULL;

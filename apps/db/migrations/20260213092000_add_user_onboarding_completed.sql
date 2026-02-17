-- +goose Up
-- Add onboarding state to users so auth can persist whether setup is complete.
ALTER TABLE users
    ADD COLUMN IF NOT EXISTS onboarding_completed BOOLEAN NOT NULL DEFAULT false;

-- +goose Down
ALTER TABLE users
    DROP COLUMN IF EXISTS onboarding_completed;

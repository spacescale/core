-- +goose Up
-- V0 schema for apps-only control plane.
-- pgcrypto enables gen_random_uuid() for DB-side UUIDs.
CREATE EXTENSION IF NOT EXISTS pgcrypto;


CREATE TABLE users
(
    id                   UUID PRIMARY KEY     DEFAULT gen_random_uuid(),
    identity_key         TEXT        NOT NULL UNIQUE CHECK (
        char_length(btrim(identity_key)) > 0
        AND char_length(identity_key) <= 512
    ),
    email                TEXT CHECK (email IS NULL OR char_length(email) <= 320),
    name                 TEXT CHECK (name IS NULL OR char_length(name) <= 255),
    avatar_url           TEXT CHECK (avatar_url IS NULL OR char_length(avatar_url) <= 2048),
    onboarding_completed BOOLEAN     NOT NULL DEFAULT false,
    created_at           TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at           TIMESTAMPTZ NOT NULL DEFAULT now()
);


-- Cascade deletes keep workspace data consistent if a user is removed.
CREATE TABLE workspaces
(
    id            UUID PRIMARY KEY     DEFAULT gen_random_uuid(),
    owner_user_id UUID        NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    name          text        NOT NULL,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (owner_user_id, name)
);

-- Cascade deletes keep project data consistent if a user is removed.
CREATE TABLE projects
(
    id            UUID PRIMARY KEY     DEFAULT gen_random_uuid(),
    workspace_id UUID        NOT NULL REFERENCES workspaces (id) ON DELETE CASCADE,
    name          TEXT        NOT NULL,
    slug          TEXT        NOT NULL UNIQUE,
    region        TEXT        NOT NULL,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Slug/subdomain are unique per project for URL {app}.{project}.{base_domain}.
CREATE TABLE apps
(
    id           UUID PRIMARY KEY     DEFAULT gen_random_uuid(),
    project_id   UUID        NOT NULL REFERENCES projects (id) ON DELETE CASCADE,
    name         TEXT,
    slug         TEXT        NOT NULL,
    subdomain    TEXT        NOT NULL,
    image_ref    TEXT        NOT NULL,
    runtime_port INT,
    status       TEXT        NOT NULL CHECK (status IN ('created', 'building', 'running', 'failed')),
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (project_id, slug),
    UNIQUE (project_id, subdomain)
);

CREATE TABLE deployments
(
    -- app_id ties deployments to apps; cascade keeps history consistent on app deletion.
    id            UUID PRIMARY KEY     DEFAULT gen_random_uuid(),
    app_id        UUID        NOT NULL REFERENCES apps (id) ON DELETE CASCADE,
    -- Deployment status reflects image-only lifecycle.
    status        TEXT        NOT NULL CHECK (status IN ('queued', 'deploying', 'running', 'failed')),
    image_ref     TEXT        NOT NULL,
    runtime_port  INT         NOT NULL,
    public_url    TEXT        NOT NULL,
    error_message TEXT,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Supports listing deployments newest-first per app.
CREATE INDEX deployments_app_id_created_at_idx
    ON deployments (app_id, created_at DESC);


CREATE TABLE app_env_vars
(
    -- app_id ties env vars to apps; cascade removes them on app deletion.
    id              UUID PRIMARY KEY     DEFAULT gen_random_uuid(),
    app_id          UUID        NOT NULL REFERENCES apps (id) ON DELETE CASCADE,
    key             TEXT        NOT NULL,
    value_encrypted TEXT        NOT NULL,
    is_secret       BOOLEAN     NOT NULL DEFAULT true,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (app_id, key)
);


CREATE TABLE registry_credentials
(
    id              UUID PRIMARY KEY     DEFAULT gen_random_uuid(),
    project_id      UUID        NOT NULL REFERENCES projects (id) ON DELETE CASCADE,
    name            TEXT        NOT NULL,
    registry_url    TEXT        NOT NULL,
    username        TEXT        NOT NULL,
    token_encrypted TEXT        NOT NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_used       TIMESTAMPTZ,
    UNIQUE (project_id, name)
);


CREATE TABLE app_registry_credentials
(
    app_id                 UUID        NOT NULL REFERENCES apps (id) ON DELETE CASCADE,
    registry_credential_id UUID        NOT NULL REFERENCES registry_credentials (id) ON DELETE CASCADE,
    created_at             TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_used              TIMESTAMPTZ,
    PRIMARY KEY (app_id, registry_credential_id)
);

-- Supports lookup by registry credential (audit/cleanup).
CREATE INDEX app_registry_credentials_registry_idx
    ON app_registry_credentials (registry_credential_id);

-- +goose Down
DROP TABLE IF EXISTS app_registry_credentials;
DROP TABLE IF EXISTS app_env_vars;
DROP TABLE IF EXISTS deployments;
DROP TABLE IF EXISTS apps;
DROP TABLE IF EXISTS registry_credentials;
DROP TABLE IF EXISTS projects;
DROP TABLE IF EXISTS workspaces;
DROP TABLE IF EXISTS users;

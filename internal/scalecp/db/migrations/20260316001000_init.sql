-- +goose Up
-- pgcrypto enables gen_random_uuid() for DB-side UUIDs.
CREATE
    EXTENSION IF NOT EXISTS pgcrypto;


CREATE TABLE users
(
    id                   UUID PRIMARY KEY     DEFAULT gen_random_uuid(),
    identity_key         TEXT        NOT NULL UNIQUE CHECK (CHAR_LENGTH(BTRIM(identity_key)) > 0 AND CHAR_LENGTH(identity_key) <= 512),
    email                TEXT CHECK (email IS NULL OR CHAR_LENGTH(email) <= 320),
    name                 TEXT CHECK (name IS NULL OR CHAR_LENGTH(name) <= 255),
    avatar_url           TEXT CHECK (avatar_url IS NULL OR CHAR_LENGTH(avatar_url) <= 2048),
    onboarding_completed BOOLEAN     NOT NULL DEFAULT FALSE,
    created_at           TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at           TIMESTAMPTZ NOT NULL DEFAULT NOW()
);


-- Cascade deletes keep workspace data consistent if a user is removed.
CREATE TABLE workspaces
(
    id            UUID PRIMARY KEY     DEFAULT gen_random_uuid(),
    owner_user_id UUID        NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    name          TEXT        NOT NULL CHECK (name = BTRIM(name)AND CHAR_LENGTH(name) BETWEEN 1 AND 255),
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (owner_user_id, name)
);

-- Cascade deletes keep project data consistent if a user is removed.
CREATE TABLE projects
(
    id           UUID PRIMARY KEY     DEFAULT gen_random_uuid(),
    workspace_id UUID        NOT NULL REFERENCES workspaces (id) ON DELETE CASCADE,
    name         TEXT        NOT NULL CHECK (CHAR_LENGTH(BTRIM(name)) > 0 AND CHAR_LENGTH(name) <= 120),
    slug         TEXT        NOT NULL UNIQUE CHECK (CHAR_LENGTH(slug) BETWEEN 1 AND 63 AND slug ~ '^[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?$'),
    region       TEXT        NOT NULL CHECK (CHAR_LENGTH(region) BETWEEN 1 AND 32 AND region ~ '^[a-z0-9](?:[a-z0-9-]{0,30}[a-z0-9])?$'),
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Supports listing projects by workspace.
CREATE INDEX projects_workspace_id_idx
    ON projects (workspace_id);


CREATE INDEX workspaces_owner_user_id_idx
    ON workspaces (owner_user_id);

-- Slug/subdomain are unique per project for URL {app}.{project}.{base_domain}.
CREATE TABLE apps
(
    id           UUID PRIMARY KEY     DEFAULT gen_random_uuid(),
    project_id   UUID        NOT NULL REFERENCES projects (id) ON DELETE CASCADE,
    name         TEXT        NOT NULL,
    slug         TEXT        NOT NULL,
    subdomain    TEXT        NOT NULL,
    image_ref    TEXT        NOT NULL,
    runtime_port INT         NOT NULL DEFAULT 8080,
    is_public    BOOLEAN     NOT NULL DEFAULT FALSE,
    status       TEXT        NOT NULL DEFAULT 'queued' CHECK (status IN ('queued', 'deploying', 'running', 'failed')),
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
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
    public_url    TEXT,
    error_message TEXT,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Supports listing deployments newest-first per app.
CREATE INDEX deployments_app_id_created_at_idx ON deployments (app_id, created_at DESC);


CREATE TABLE app_env_vars
(
    -- app_id ties env vars to apps; cascade removes them on app deletion.
    id              UUID PRIMARY KEY     DEFAULT gen_random_uuid(),
    app_id          UUID        NOT NULL REFERENCES apps (id) ON DELETE CASCADE,
    key             TEXT        NOT NULL,
    value_encrypted TEXT        NOT NULL,
    cipher_version  TEXT GENERATED ALWAYS AS (split_part(value_encrypted, ':', 1)) STORED,
    cipher_algo     TEXT GENERATED ALWAYS AS (split_part(value_encrypted, ':', 2)) STORED,
    cipher_key_id   TEXT GENERATED ALWAYS AS (split_part(value_encrypted, ':', 3)) STORED,
    is_secret       BOOLEAN     NOT NULL DEFAULT TRUE,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (app_id, key)
);

CREATE INDEX app_env_vars_cipher_claim_idx ON app_env_vars (cipher_key_id, created_at) WHERE cipher_version = 'v1' AND cipher_algo = 'aesgcm';


-- PURPOSE: The globally shared memory for the SpaceScale Control Plane.
CREATE TABLE systems_configs (
    key TEXT PRIMARY KEY CHECK (CHAR_LENGTH(BTRIM(key)) > 0),
    value TEXT NOT NULL, -- actual config value, might also be encrypted if secret is true
    is_secret BOOLEAN NOT NULL DEFAULT FALSE,
    description TEXT,-- human readable desc
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
    -- expected day 0 data might be ssh public and private key pair used to boostrap nodes automatically
    -- after purchase from IaaS provider
);

-- list of IaaS providers and their api token spacescale depends on, metal only
CREATE TABLE providers (
        id TEXT PRIMARY KEY CHECK (CHAR_LENGTH(BTRIM(id)) > 0), --no uuid provider basically less than 10
        name TEXT UNIQUE NOT NULL, --  'hetzner' europe and us', 'ovh for canada ', 'vultr' and soon spacescale colo
        api_token_encrypted TEXT NOT NULL,
        is_active BOOLEAN DEFAULT TRUE,
        created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
        updated_at TIMESTAMPTZ      NOT NULL DEFAULT NOW()
);

-- Table of all Baremetal servers managed by scalecp
CREATE TABLE metals (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    provider_id TEXT NOT NULL REFERENCES providers(id) ON DELETE RESTRICT,
    provider_server_id TEXT NOT NULL,
    primary_ipv4 TEXT UNIQUE NOT NULL,
    primary_ipv6 TEXT UNIQUE,
    host_os_family TEXT,         -- ubuntu, debian, rocky, etc
    host_os_version TEXT,        -- 24.04, 12, 9.4
    host_image_ref TEXT,         -- provider image slug / internal image version
    region TEXT NOT NULL, --normalized region
    provider_location TEXT NOT NULL,  -- raw provider location code like FSN1-DC10 or gra1
    tier_target TEXT NOT NULL CHECK (tier_target IN ('shared', 'dedicated')), --- for isolating shared/dedicated
    total_cpu_core INT NOT NULL CHECK (total_cpu_core > 0), -- cores are not threads we can get that directly from iaas provider
    total_threads INT NOT NULL DEFAULT 0 CHECK (total_threads >= 0), -- updated by scaled once node becomes active
    total_ram_mb BIGINT NOT NULL DEFAULT 0 CHECK (total_ram_mb >= 0), -- updated by scaled once node becomes active
    total_disk_mb BIGINT NOT NULL DEFAULT 0 CHECK (total_disk_mb >= 0), --updated by scaled once node becomes active
    status TEXT NOT NULL DEFAULT 'provisioning' CHECK (status IN ('provisioning', 'active', 'retired', 'faulty', 'maintenance')), -- node becomes active if daemon connects as well
    bootstrap_token_hash TEXT UNIQUE, -- inject  token at init bootstrap, used by daemon to prove identity
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Index to instantly find a specific server when a provider sends an API webhook/event
CREATE INDEX metals_provider_server_idx ON metals (provider_id, provider_server_id);

-- Durable scaled daemon, and allocation ledger
CREATE TABLE scaled
(
    id               TEXT PRIMARY KEY CHECK (id = BTRIM(id) AND CHAR_LENGTH(id) BETWEEN 1 AND 255),
    version TEXT NOT NULL, -- running daemon version
    total_allocated_vms_threads INT NOT NULL DEFAULT 0 CHECK (total_allocated_vms_threads >= 0),
    total_allocated_vms_ram_mb BIGINT NOT NULL DEFAULT 0 CHECK (total_allocated_vms_ram_mb >= 0),
    total_allocated_vm_disk_mb BIGINT NOT NULL DEFAULT 0 CHECK (total_allocated_vm_disk_mb >= 0),
    metal_id UUID NOT NULL UNIQUE REFERENCES metals(id) ON DELETE CASCADE,
    created_at       TIMESTAMPTZ      NOT NULL DEFAULT NOW(),
    updated_at       TIMESTAMPTZ      NOT NULL DEFAULT NOW()
);


CREATE TABLE registry_credentials
(
    id              UUID PRIMARY KEY     DEFAULT gen_random_uuid(),
    project_id      UUID        NOT NULL REFERENCES projects (id) ON DELETE CASCADE,
    name            TEXT        NOT NULL,
    registry_url    TEXT        NOT NULL,
    username        TEXT        NOT NULL,
    token_encrypted TEXT        NOT NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_used       TIMESTAMPTZ,
    UNIQUE (project_id, name)
);


CREATE TABLE app_registry_credentials
(
    app_id                 UUID        NOT NULL REFERENCES apps (id) ON DELETE CASCADE,
    registry_credential_id UUID        NOT NULL REFERENCES registry_credentials (id) ON DELETE CASCADE,
    created_at             TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_used              TIMESTAMPTZ,
    PRIMARY KEY (app_id, registry_credential_id)
);

-- Supports lookup by registry credential (audit/cleanup).
CREATE INDEX app_registry_credentials_registry_idx ON app_registry_credentials (registry_credential_id);

-- +goose Down
DROP TABLE IF EXISTS app_registry_credentials;
DROP TABLE IF EXISTS app_env_vars;
DROP TABLE IF EXISTS scaled;
DROP TABLE IF EXISTS metals;
DROP TABLE IF EXISTS providers;
DROP TABLE IF EXISTS systems_configs;
DROP TABLE IF EXISTS deployments;
DROP TABLE IF EXISTS apps;
DROP TABLE IF EXISTS registry_credentials;
DROP TABLE IF EXISTS projects;
DROP TABLE IF EXISTS workspaces;
DROP TABLE IF EXISTS users;
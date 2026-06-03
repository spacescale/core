-- +goose Up
-- pgcrypto gives the schema a safe database side uuid generator.
CREATE
    EXTENSION IF NOT EXISTS pgcrypto;


-- users holds the durable authenticated identity record for one person.
-- Everything else in the tenant model hangs off this table through ownership.
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

-- workspaces are the billing and isolation boundary for customer resources.
-- If an account is removed, its workspaces disappear with it.
CREATE TABLE workspaces
(
    id            UUID PRIMARY KEY     DEFAULT gen_random_uuid(),
    owner_user_id UUID        NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    name          TEXT        NOT NULL CHECK (name = BTRIM(name)AND CHAR_LENGTH(name) BETWEEN 1 AND 255),
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (owner_user_id, name)
);
-- projects group deployable products inside one workspace.
-- A project gives apps and future managed resources a stable home.
CREATE TABLE projects
(
    id           UUID PRIMARY KEY     DEFAULT gen_random_uuid(),
    workspace_id UUID        NOT NULL REFERENCES workspaces (id) ON DELETE CASCADE,
    name         TEXT        NOT NULL CHECK (CHAR_LENGTH(BTRIM(name)) > 0 AND CHAR_LENGTH(name) <= 120),
    slug         TEXT        NOT NULL UNIQUE CHECK (CHAR_LENGTH(slug) BETWEEN 1 AND 63 AND slug ~ '^[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?$'),
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
-- projects are listed by workspace constantly, so keep that path indexed.
CREATE INDEX projects_workspace_id_idx
    ON projects (workspace_id);


-- users list and resolve workspaces by owner all the time.
CREATE INDEX workspaces_owner_user_id_idx
    ON workspaces (owner_user_id);

-- apps are the long lived product object customers think about.
-- An app survives across many deployments and across many microvms over time.
CREATE TABLE apps
(
    id           UUID PRIMARY KEY     DEFAULT gen_random_uuid(),
    project_id   UUID        NOT NULL REFERENCES projects (id) ON DELETE CASCADE,
    name         TEXT        NOT NULL,
    slug         TEXT        NOT NULL,
    subdomain    TEXT        NOT NULL,
    image_ref    TEXT        NOT NULL,
    target_replicas INT      NOT NULL DEFAULT 1 CHECK (target_replicas > 0),
    primary_region TEXT      NOT NULL DEFAULT 'us-east',
    runtime_port INT         NOT NULL DEFAULT 8080,
    is_public    BOOLEAN     NOT NULL DEFAULT FALSE,
    status       TEXT        NOT NULL DEFAULT 'queued' CHECK (status IN ('queued', 'deploying', 'running', 'failed')),
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (project_id, slug),
    UNIQUE (project_id, subdomain)
);

-- deployments capture one concrete release of an app.
-- This is the row that rolling updates and replica groups should hang off.
CREATE TABLE deployments
(
    id            UUID PRIMARY KEY     DEFAULT gen_random_uuid(),
    app_id        UUID        NOT NULL REFERENCES apps (id) ON DELETE CASCADE,
    status        TEXT        NOT NULL CHECK (status IN ('queued', 'deploying', 'running', 'failed')),
    image_ref     TEXT        NOT NULL,
    runtime_port  INT         NOT NULL,
    public_url    TEXT,
    error_message TEXT,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
-- Deployments are read newest first for app release history and rollout logic.
CREATE INDEX deployments_app_id_created_at_idx ON deployments (app_id, created_at DESC);


-- app_env_vars stores encrypted environment variables for one app.
-- Values never land in plaintext in Postgres.
CREATE TABLE app_env_vars
(
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



-- nodes is the inventory of real schedulable hosts managed by the platform.
-- This row becomes the durable runtime identity once scaled boots and joins the
-- system. Provider details still live here because this is the only host record.
CREATE TABLE nodes (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    provider TEXT NOT NULL CHECK (provider IN ('ovh', 'colo')),
    provider_server_id TEXT NOT NULL,
    primary_ipv4 TEXT UNIQUE NOT NULL,
    primary_ipv6 TEXT UNIQUE,
    region TEXT NOT NULL,
    provider_location TEXT NOT NULL,
    total_cores INT NOT NULL DEFAULT 0 CHECK (total_cores >= 0),
    total_ram_mb BIGINT NOT NULL DEFAULT 0 CHECK (total_ram_mb >= 0),
    total_disk_mb BIGINT NOT NULL DEFAULT 0 CHECK (total_disk_mb >= 0),
    status TEXT NOT NULL DEFAULT 'provisioning' CHECK (status IN ('provisioning', 'active', 'retired', 'faulty', 'maintenance')),
    bootstrap_token_hash TEXT UNIQUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- microvms is the generic inventory of running compute.
-- Each row is one Firecracker process on one host.
-- Ownership is polymorphic on purpose so future resources do not force another
-- schema rewrite. App backed microvms point at deployments today. Internal
-- system workloads and future managed products can point somewhere else later.
CREATE TABLE microvms
(
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id UUID NOT NULL REFERENCES workspaces (id) ON DELETE CASCADE,
    resource_type TEXT NOT NULL CHECK (resource_type = BTRIM(resource_type) AND CHAR_LENGTH(resource_type) BETWEEN 1 AND 64),
    resource_id UUID,
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
    node_id UUID REFERENCES nodes(id) ON DELETE RESTRICT,
    region TEXT NOT NULL,
    vcpu INT NOT NULL CHECK (vcpu > 0),
    ram_mb BIGINT NOT NULL CHECK (ram_mb > 0),
    cpu_mode TEXT NOT NULL CHECK (cpu_mode IN ('shared', 'pinned')),
    root_disk_mb BIGINT NOT NULL CHECK (root_disk_mb > 0),
    volume_mb BIGINT NOT NULL DEFAULT 0 CHECK (volume_mb >= 0),
    status TEXT NOT NULL DEFAULT 'queued' CHECK (status IN ('queued', 'assigned', 'starting', 'running', 'stopping', 'destroyed', 'failed')),
    error_message TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Workspace lookups drive tenancy, billing, and generic cleanup.
CREATE INDEX microvms_workspace_id_idx ON microvms (workspace_id);

-- Generic resource ownership lookups let one controller find all microvms for
-- one deployment or any future product resource.
CREATE INDEX microvms_resource_idx ON microvms (resource_type, resource_id);

-- Placement and reconciliation both need fast scans by region and lifecycle.
CREATE INDEX microvms_region_status_idx ON microvms (region, status);


-- registry_credentials stores encrypted pull credentials owned by one project.
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


-- app_registry_credentials connects an app to the registry secret it uses.
CREATE TABLE app_registry_credentials
(
    app_id                 UUID        NOT NULL REFERENCES apps (id) ON DELETE CASCADE,
    registry_credential_id UUID        NOT NULL REFERENCES registry_credentials (id) ON DELETE CASCADE,
    created_at             TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_used              TIMESTAMPTZ,
	PRIMARY KEY (app_id, registry_credential_id)
);

-- Registry first lookups are needed for audit and cleanup work.
CREATE INDEX app_registry_credentials_registry_idx ON app_registry_credentials (registry_credential_id);

-- +goose Down
DROP TABLE IF EXISTS app_registry_credentials;
DROP TABLE IF EXISTS app_env_vars;
DROP TABLE IF EXISTS microvms;
DROP TABLE IF EXISTS nodes;
DROP TABLE IF EXISTS deployments;
DROP TABLE IF EXISTS apps;
DROP TABLE IF EXISTS registry_credentials;
DROP TABLE IF EXISTS projects;
DROP TABLE IF EXISTS workspaces;
DROP TABLE IF EXISTS users;

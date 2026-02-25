-- +goose Up
-- Harden project fields used in URLs/DNS and cap user-controlled text sizes.
ALTER TABLE projects
    ADD CONSTRAINT projects_name_required_and_max_len CHECK (
        char_length(btrim(name)) > 0
            AND char_length(name) <= 120
        ),
    ADD CONSTRAINT projects_slug_dns_label CHECK (
        char_length(slug) BETWEEN 1 AND 63
            AND slug ~ '^[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?$'
        ),
    ADD CONSTRAINT projects_region_dns_label CHECK (
        char_length(region) BETWEEN 1 AND 32
            AND region ~ '^[a-z0-9](?:[a-z0-9-]{0,30}[a-z0-9])?$'
        );

-- +goose Down
ALTER TABLE projects
    DROP CONSTRAINT IF EXISTS projects_region_dns_label,
    DROP CONSTRAINT IF EXISTS projects_slug_dns_label,
    DROP CONSTRAINT IF EXISTS projects_name_required_and_max_len;

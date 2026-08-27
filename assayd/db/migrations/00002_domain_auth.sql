-- +goose Up
CREATE TABLE projects (
    id uuid PRIMARY KEY,
    name text NOT NULL UNIQUE CHECK (btrim(name) <> ''),
    judge_config jsonb,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE api_keys (
    id uuid PRIMARY KEY,
    project_id uuid NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    name text NOT NULL CHECK (btrim(name) <> ''),
    key_hash bytea NOT NULL UNIQUE CHECK (octet_length(key_hash) = 32),
    key_prefix text NOT NULL CHECK (char_length(key_prefix) = 8),
    last_used_at timestamptz,
    revoked_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE applications (
    id uuid PRIMARY KEY,
    project_id uuid NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    name text NOT NULL CHECK (btrim(name) <> ''),
    slug text NOT NULL CHECK (btrim(slug) <> ''),
    config jsonb NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(config) = 'object'),
    auto_score_scorers text[] NOT NULL DEFAULT '{}',
    target_endpoint jsonb,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (project_id, slug)
);

CREATE INDEX api_keys_project_id_idx ON api_keys(project_id);
CREATE INDEX applications_project_id_idx ON applications(project_id);

-- +goose Down
DROP TABLE applications;
DROP TABLE api_keys;
DROP TABLE projects;

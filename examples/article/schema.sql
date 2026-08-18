-- Example PostgreSQL schema for article resource and CMS core tables

-- Content Table: categories
CREATE TABLE IF NOT EXISTS categories (
    id UUID PRIMARY KEY,
    name VARCHAR(255) NOT NULL,
    slug VARCHAR(255) NOT NULL UNIQUE
);

-- Content Table: articles (Owned by the application project)
CREATE TABLE IF NOT EXISTS articles (
    id UUID PRIMARY KEY,
    title VARCHAR(255) NOT NULL,
    body TEXT,
    cover_image_id UUID,
    category_id UUID REFERENCES categories(id),

    -- Explicit single dependency reference (for DAG / Gantt / Sequencing)
    depends_on_id UUID REFERENCES articles(id),

    -- Schedule fields
    published_at TIMESTAMPTZ,
    finished_at TIMESTAMPTZ,

    -- Managed Lifecycle columns (draft -> published -> finished)
    cms_status TEXT NOT NULL DEFAULT 'draft',
    cms_version BIGINT NOT NULL DEFAULT 1,

    -- Timestamps
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- CMS Internal Table: Audit Logs
CREATE TABLE IF NOT EXISTS cms_audit_logs (
    id UUID PRIMARY KEY,
    actor TEXT NOT NULL,
    actor_type TEXT NOT NULL,
    operation TEXT NOT NULL,
    resource TEXT NOT NULL,
    resource_id TEXT NOT NULL,
    request_id TEXT NOT NULL,
    timestamp TIMESTAMPTZ NOT NULL,
    changes_json TEXT
);

-- CMS Internal Table: Revisions
CREATE TABLE IF NOT EXISTS cms_revisions (
    id UUID PRIMARY KEY,
    resource TEXT NOT NULL,
    resource_id TEXT NOT NULL,
    version BIGINT NOT NULL,
    schema_version TEXT,
    snapshot_json TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL,
    actor TEXT NOT NULL
);

-- CMS Internal Table: API Keys
CREATE TABLE IF NOT EXISTS cms_api_keys (
    id UUID PRIMARY KEY,
    name VARCHAR(255) NOT NULL,
    token_hash VARCHAR(64) NOT NULL UNIQUE,
    permission VARCHAR(32) NOT NULL DEFAULT 'read_write',
    is_active BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    revoked_at TIMESTAMPTZ
);

-- CMS Internal Table: Media Metadata
CREATE TABLE IF NOT EXISTS cms_media (
    id UUID PRIMARY KEY,
    filename VARCHAR(255) NOT NULL,
    filepath VARCHAR(512) NOT NULL,
    mime_type VARCHAR(128) NOT NULL,
    size_bytes BIGINT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

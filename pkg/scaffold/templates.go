package scaffold

const ArticleYAML = `resource: article
storage:
  table: articles
lifecycle:
  mode: managed
  status_column: cms_status
  version_column: cms_version
fields:
  id:
    type: uuid
    column: id
    readonly: true
  title:
    type: string
    column: title
    required: true
  body:
    type: text
    column: body
  cover_image:
    type: media
    column: cover_image_id
  category_id:
    type: uuid
    column: category_id
  depends_on:
    type: reference
    column: depends_on_id
    resource: article
  published_at:
    type: datetime
    column: published_at
  finished_at:
    type: datetime
    column: finished_at
`

const MigrationSQL = `-- Initial migration for article resource and CMS core tables

CREATE TABLE IF NOT EXISTS articles (
    id UUID PRIMARY KEY,
    title VARCHAR(255) NOT NULL,
    body TEXT,
    cover_image_id UUID,
    category_id UUID,

    -- Explicit single dependency reference (for DAG / Kanban / Gantt)
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
`

const DotEnvExample = `# PostgreSQL Database Connection URL (Replace credentials for production)
DATABASE_URL=postgres://postgres:CHANGE_THIS_IN_PRODUCTION@localhost:5432/cms_db?sslmode=disable

# Directory containing YAML Resource Definitions
CMS_SCHEMA_DIR=./resources

# HTTP Server Host and Port
CMS_HOST=127.0.0.1
PORT=8080

# Optional Static API Key and Admin Basic Auth (Set strong credentials in production)
# CMS_API_KEY=
# CMS_ADMIN_USER=admin
# CMS_ADMIN_PASSWORD=CHANGE_THIS_STRONG_PASSWORD
`

const QuickStartREADME = `# gohcms Project Quickstart

This project was initialized by cms init.

## Quickstart Guide

### 1. Run in Demo Mode (No Database Required)
` + "```bash" + `
cms serve --demo --schema-dir ./resources --port 8080
` + "```" + `
Open http://localhost:8080/ to explore the Table, Kanban, Timeline, Media Library, and API Keys Admin UI.

### 2. Connect to PostgreSQL
` + "```bash" + `
# 1. Copy environment variables
cp .env.example .env

# 2. Apply migration to your PostgreSQL database
psql $DATABASE_URL -f migrations/001_create_articles.sql

# 3. Validate schema contracts
cms validate --db "$DATABASE_URL" --schema-dir ./resources

# 4. Start production server
cms serve --db "$DATABASE_URL" --schema-dir ./resources --port 8080
` + "```" + `

### 3. Export OpenAPI 3.1 Specification
` + "```bash" + `
cms openapi export --schema-dir ./resources -o openapi.json
` + "```" + `
`

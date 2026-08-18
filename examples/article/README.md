# Article CMS Example (Phase 0.1 Validation Spike)

This example demonstrates how to define a resource with `gohcms` and validate it against a PostgreSQL database schema or standalone in Go code.

## Files
- [`article.yaml`](./article.yaml): Resource Definition for the `article` resource.
- [`schema.sql`](./schema.sql): PostgreSQL DDL for the `articles` table matching the resource definition.
- [`main.go`](./main.go): Standalone Go program demonstrating `gohcms` as an embedded core library.

## How to Test

### 1. Launch Admin Web UI in Demo Mode (No Database / No Docker Needed!)
```bash
# Start the Go + htmx Admin UI with built-in in-memory mock data
go run ./cmd/cms serve --demo --schema-dir examples/article --port 8080
```
Open [http://localhost:8080/](http://localhost:8080/) in your browser to experience:
- **📌 Kanban Board**: Drag & drop cards between Draft, Published, and Finished columns.
- **⏳ Timeline / Gantt**: Visual schedule and DAG prerequisite dependency flow.
- **📋 Table View**: Full CRUD operations, inline actions, and quick filters.
- **📜 Audit & Revisions**: Modal displaying transaction logs and version snapshots.
- **📖 OpenAPI 3.1 Spec**: Access live API spec at [http://localhost:8080/api/openapi.json](http://localhost:8080/api/openapi.json).

### 2. Export OpenAPI 3.1 Specification
```bash
# Export as JSON
go run ./cmd/cms openapi export --file examples/article/article.yaml -o openapi.json

# Export as YAML
go run ./cmd/cms openapi export --file examples/article/article.yaml --format yaml
```

### 3. Headless REST API Endpoints
```bash
# Get published articles
curl http://localhost:8080/api/article

# Get all articles including drafts and finished
curl "http://localhost:8080/api/article?status=all"

# Create a new article
curl -X POST http://localhost:8080/api/article \
  -H "Content-Type: application/json" \
  -d '{"id":"art-api","title":"Headless API Article","body":"Created via REST"}'

# Publish an article
curl -X POST http://localhost:8080/api/article/art-api/publish

# Finish/archive an article
curl -X POST http://localhost:8080/api/article/art-api/finish
```

### 2. Standalone Core Library Run
```bash
go run examples/article/main.go
```

### 3. Validate & Run with Real PostgreSQL Database
If you have PostgreSQL running:
```bash
# Apply schema to PostgreSQL
psql $DATABASE_URL -f examples/article/schema.sql

# Validate schema contract
go run ./cmd/cms validate --db "$DATABASE_URL" --file examples/article/article.yaml

# Start Admin UI with PostgreSQL
go run ./cmd/cms serve --db "$DATABASE_URL" --schema-dir examples/article --port 8080
```
Open [http://localhost:8080/](http://localhost:8080/) to experience:
- **Table View**: Full content CRUD with status badges and quick action buttons.
- **Kanban Board**: Drag-and-drop cards between Draft, Published, and Finished columns.
- **Timeline View**: Visual schedule and DAG dependency hierarchy.
- **Audit & Revisions**: Modal displaying transaction audit trail and version snapshots.

Output on success:
```text
✓ article (table exists)
✓ article.title -> articles.title (character varying NOT NULL)
✓ article.body -> articles.body (text)
✓ article.category_id -> articles.category_id (uuid)
✓ article.id -> articles.id (uuid NOT NULL)
✓ article (lifecycle status column (text))
✓ article (lifecycle version column (bigint))
```

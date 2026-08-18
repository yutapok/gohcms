Agent-Native Headless CMS — Revised PRD

1. Product Vision

Go + htmxを中心に構築する、軽量で運用しやすいOSS Headless CMS。

Strapiの優れたDeveloper Experienceである、

- Resourceを定義すると管理画面が使える
- CRUD APIが利用できる
- Draft / Publishが扱える
- CMSとしてすぐ開始できる

という利点を残しつつ、以下の運用課題を解消する。

- CMSアップデートとContent Schema Migrationの密結合
- CMSによるDatabase Schema ownership
- Node / JavaScript dependency treeの運用負荷
- Admin / API / Workerの強い結合
- Audit / Revisionの後付け
- API Contractの不透明さ
- Relationの複雑化

長期的には、Human / Application / AI Agentが共通のContent ContractとCore Serviceを利用できる、Agent-nativeなContent Platformを目指す。

ただしMVPではAgent機能を主目的としない。

初期の価値は以下に集中する。

«A boring, upgrade-safe Headless CMS for Go teams.»

---

2. Core Principles

2.1 Your database, your migrations

Database SchemaとMigrationはプロジェクト側が所有する。

CMSはContent Tableを勝手に変更しない。

CMSアップグレードによるMigrationは、原則としてCMS管理テーブルのみを対象とする。

例：

cms_users
cms_roles
cms_audit_logs
cms_revisions

Content Table：

articles
products
authors
categories

はプロジェクト側の所有物とする。

---

2.2 Resource Definition is a CMS Contract

Resource DefinitionはDatabase Schemaそのものではない。

既存のDatabase SchemaをCMS上でどのように扱うかを定義する。

例：

resource: article

storage:
  table: articles

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

  author:
    type: reference
    column: author_id
    resource: author

Resource Definitionから以下を提供する。

Admin Form
Validation
REST API Schema
OpenAPI

将来的にはMCP Tool Schemaも生成する。

---

2.3 Database Schema is storage truth

Resource DefinitionとDatabase Schemaは別物として扱う。

ただし二重管理によるDriftを防ぐため、CMS CLIがDatabase Schemaを検査する。

cms validate

例：

✓ articles.id      uuid
✓ articles.title   text NOT NULL

ERROR:
article.author expects articles.author_id UUID
actual database type is BIGINT

CMSはSchema変更を勝手に適用しない。

将来的には、

cms schema diff
cms schema suggest-migration

によってDDL提案を行うことは可能とする。

適用は利用者が判断する。

---

2.4 CMS conventions are explicit and opt-in

CMS固有機能を使う場合、必要なStorage Conventionを明示する。

例えばDraft / PublishをCMS管理するResourceでは、

lifecycle:
  mode: managed
  status_column: cms_status
  version_column: cms_version

とする。

対応するTable例：

CREATE TABLE articles (
    id UUID PRIMARY KEY,
    title TEXT NOT NULL,
    body TEXT,

    cms_status TEXT NOT NULL DEFAULT 'draft',
    cms_version BIGINT NOT NULL DEFAULT 1,

    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL
);

既存DatabaseでLifecycleをCMSに管理させたくない場合、

lifecycle:
  mode: none

を選択できる。

将来的には、

lifecycle:
  mode: external

も検討する。

---

2.5 One Core, multiple interfaces

Business LogicはCore Serviceに集約する。

Admin
REST API
Future MCP
Future Worker
       |
       v
ContentService
       |
ContentRepository
       |
PostgreSQL

Core Serviceは以下を知らない。

- HTTP
- htmx
- MCP
- deployment topology

MVPではRemote Core APIや分散用Client abstractionは実装しない。

---

2.6 Server-rendered first, JavaScript where needed

Admin UIはGo templates + htmxを基本とする。

ただしJavaScriptを禁止しない。

以下のような高度なInteractionには局所的なJavaScript Componentを利用してよい。

Rich Text Editor
Reference Picker
Media Picker
Array Editor
JSON Editor

目的はJavaScriptゼロではない。

目的は、

«Node SPA applicationをCMS運用の必須条件にしない»

ことである。

---

2.7 Explicit over magical

以下を暗黙化しすぎない。

Database Migration
Relation
Storage Mapping
Lifecycle
Workflow

CMSが自動的にDB構造や巨大なRelation Graphを作る設計を避ける。

---

3. Problems to Solve

3.1 Upgrade coupling

CMS Version UpdateによってContent Table Migrationを要求される状態を避ける。

Target:

CMS minor upgrade
        |
        +--> CMS internal migration: possible
        |
        +--> Content table migration: normally zero

---

3.2 Schema ownership

Content Schemaの変更はGit管理されたMigrationとしてProject側が行う。

CMSはそれをResource Definition経由で利用する。

---

3.3 Schema drift

Database SchemaとResource Definitionの不一致をCLIで検知する。

---

3.4 JavaScript operational burden

Admin RuntimeはGo binaryで完結させる。

局所的なJavaScript依存は許容するが、Node runtimeを本番運用に必須としない。

---

3.5 Audit

すべてのContent MutationをCore Service経由にする。

Audit Logは後付けPluginではなくCore機能とする。

---

3.6 API Contract

Resource DefinitionからOpenAPI 3.1を生成する。

OpenAPIをClient Generationおよび将来のAgent Integrationの基本Contractとする。

---

3.7 Relation complexity

MVPではRelationをReferenceに限定する。

Article.author_id -> Author.id

Implicit Many-to-Manyや任意深度Graph Traversalは提供しない。

---

4. Target Users

4.1 Go Development Teams

Node系CMSを避けたいチーム。

4.2 Small Product Teams

Single Binary + PostgreSQLで小さく始めたいチーム。

4.3 Existing Strapi Users

StrapiのDeveloper Experienceは好きだが、

Upgrade
Migration
Dependency management
Runtime coupling
Relation complexity

に課題を感じているチーム。

4.4 Future Agent-oriented Teams

将来的にCoding Agent / Content Agent / SRE AgentからCMSを調査・操作したいチーム。

---

5. MVP Product Position

MVPでは以下の価値だけに集中する。

Single Go Binary
PostgreSQL
Resource Definition
Schema Drift Validation
Generated Admin
Generated REST
Generated OpenAPI
Audit
Draft / Publish

MCP、分散化、高度なWorkflowは初期差別化として実装しない。

---

6. MVP Architecture

                     Resource Definition
                             |
                  +----------+----------+
                  |                     |
             Validation            Admin Metadata
                  |                     |
                  +----------+----------+
                             |
                       ContentService
                             |
                     ContentRepository
                             |
                         PostgreSQL

                  +----------+----------+
                  |                     |
                Admin                 REST
             Go + htmx              OpenAPI

Single Processとして動作する。

+-----------------------------------+
|                cms                |
|                                   |
| Admin                             |
| REST API                          |
| Core Service                      |
| Schema Validation                 |
+-----------------+-----------------+
                  |
              PostgreSQL

---

7. Resource Definition

MVPで扱うField Type：

uuid
string
text
integer
float
boolean
datetime
enum
json
reference

例：

resource: article

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

  category:
    type: reference
    column: category_id
    resource: category

---

8. Database Validation

CMSはPostgreSQL metadataを読み取り、Resource Definitionと比較する。

検査対象：

Table existence
Column existence
Column type
Nullable
Unique where applicable
Reference target existence
Lifecycle columns

MVPではDB Migrationの自動実行はしない。

---

9. Content Lifecycle

MVPでは2モードのみ。

none

通常CRUD。

lifecycle:
  mode: none

managed

lifecycle:
  mode: managed
  status_column: cms_status
  version_column: cms_version

状態：

draft
published

Custom WorkflowはMVP対象外。

---

10. Revision

RevisionはAuditとは別概念。

MVPでは以下に限定する。

Snapshot保存
Revision一覧
Diff表示

Storage例：

cms_revisions
-------------
resource
resource_id
version
schema_version
snapshot_json
created_at
actor

MVPでは古いSchema Versionからの自動Restoreを保証しない。

Current Resource DefinitionでValidation可能なRevisionのみRestore候補とする。

Restore自体はv0.1必須ではない。

---

11. Audit

Mutation時に以下を記録する。

actor
operation
resource
resource_id
request_id
timestamp

Actor：

user
api_client
system

将来的に、

agent

を追加する。

MVP Transaction：

BEGIN

Content Mutation
Revision INSERT
Audit INSERT

COMMIT

OutboxはMVPから除外する。

---

12. Admin UI

Go templates + htmxを基本とする。

MVP：

Resource navigation
Content list
Create
Edit
Delete
Publish
Unpublish
Revision list
Audit list

初期Widget：

text
textarea
number
checkbox
datetime
select
json
reference

Reference Pickerは検索可能な軽量UIとする。

必要に応じて局所的なJavaScriptを許容する。

---

13. REST API

MVP：

GET    /api/{resource}
GET    /api/{resource}/{id}
POST   /api/{resource}
PATCH  /api/{resource}/{id}
DELETE /api/{resource}/{id}

Query：

pagination
basic sort
basic filter
explicit reference include

任意深度populateは提供しない。

---

14. OpenAPI

Resource DefinitionからOpenAPI 3.1を生成する。

cms openapi export

OpenAPIを以下の基本Contractにする。

API Documentation
TypeScript Client Generation
Go Client Generation
Future MCP Gateway
Future Agent Tooling

MVPでは独自MCP Serverを実装しない。

---

15. Relation Model

MVPではReferenceのみ。

Article.category_id
Article.author_id

Many-to-ManyはMVP対象外。

将来的にはExplicit Link Resourceとして追加する。

ArticleTag

article_id
tag_id
position

---

16. Customization

OSS Distributionとしてそのまま利用可能にする。

cms init
cms validate
cms serve

高度な利用者はGo libraryとして組み込める設計を維持する。

ただしMVPでは拡張APIを過剰設計しない。

最低限のDI対象：

ContentRepository
Authorizer

を候補とする。

Go runtime plugin機構は利用しない。

---

17. Strapi Compatibility

Strapi Importer完成版はMVP対象外。

ただしResource Definitionの設計検証のため、Phase 0から実在するStrapi Schemaを利用する。

最低3パターン：

Simple Blog
Relations + Media
Components + Dynamic Zones

確認目的：

Resource Definitionで表現可能か
どこからUnsupportedになるか
どの情報をImporterで変換可能か

将来的には、

cms import strapi

を提供する。

ただし目標は100%互換ではなく、

«自動変換可能部分とManual Review領域を明示する»

ことである。

---

18. AI / MCP Strategy

Agent-nativeは長期ビジョンとして維持する。

MVPではOpenAPIをAgent Integrationの入口とする。

Agent
  |
MCP/OpenAPI Gateway
  |
OpenAPI
  |
REST API
  |
CMS Core

Native MCPは、REST/OpenAPIで表現しづらいControl Plane機能が必要になった時点で追加する。

候補：

schema.drift
audit.search
content.validate
revision.inspect
system.capabilities
health.get

長期的には、

Content Agent
Coding Agent
SRE Agent
Security Agent

から利用可能にする。

---

19. MVP Scope

Include

PostgreSQL

Single Go binary

Resource Definition

Database schema validation

Basic field types

Reference

ContentService

Generic PostgreSQL Repository

Admin CRUD

Draft / Published

Audit

Basic Revision history

REST API

OpenAPI 3.1

Exclude

Native MCP Server
Distributed Runtime
Remote CoreClient
Transactional Outbox
Media Library
Many-to-Many
Nested Components
Dynamic Zones
i18n
GraphQL
Advanced RBAC
Custom Workflow
Scheduled Publish
Webhook Worker
Multiple Databases
Plugin Marketplace

---

20. MVP Success Criteria

Time to First Content

cms init
  |
Resource Definition
  |
cms validate
  |
cms serve
  |
Create Article
  |
GET /api/articles/:id

10分以内を目標とする。

Time to New Resource

Resource Definition追加後、

Admin CRUD
REST
OpenAPI

が追加実装なしで利用できる。

Upgrade Safety

CMS minor updateによるContent Table Migration：

0

を基本とする。

Drift Detection

Resource DefinitionとDatabase Schemaの不整合を起動前またはCIで検出できる。

Node Runtime

標準的な本番運用ではNode.js runtimeを要求しない。

---

21. Non-goals

MVPでは以下を目指さない。

Strapi完全互換
No-code Schema Builder
General-purpose ORM
General-purpose Workflow Engine
General-purpose AI Agent Platform
General-purpose Observability Platform
Microservices Platform

---

22. Long-term Direction

Foundation：

Predictable CMS
Single Binary
Upgrade-safe
User-owned schema

Differentiation：

Resource Contract
Generated Admin
Generated OpenAPI
Low operational burden

Future Moat：

Agent-native control plane
Development agents
SRE agents
Data quality agents
Security agents

長期的には、

«Headless CMS with an Agent-Native Control Plane»

を目指すが、Agent機能のためにCMSとしての基本品質を犠牲にしない。

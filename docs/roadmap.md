Agent-Native Headless CMS — Revised Development Roadmap

開発原則

最初の目標はFrameworkを作ることではない。

«実案件で使える最小のCMSを完成させる。»

将来の分散化・MCP・高度な拡張性のための実装は前倒ししない。

ただし、後から変更すると大きく痛む以下の境界だけは早期に固める。

Resource Definition
ContentService
Repository boundary
Database ownership
Lifecycle convention
Audit transaction

---

Phase 0 — Validation Spike

Goal

最大の設計リスクを実コードと実在Schemaで検証する。

期間目安：

1〜2週間

ただし期間よりExit Criteriaを優先する。

---

0.1 Article Vertical Slice

以下のTableを用意する。

articles
--------
id
title
body
category_id
cms_status
cms_version
created_at
updated_at

Resource Definition：

resource: article

最低限、

Resource Definition
       |
DB Validation
       |
Repository
       |
ContentService
       |
Admin
       |
REST

まで一本で貫通させる。

---

0.2 Real Strapi Schema Study

Importerはまだ作らない。

実在するStrapi Schemaを最低3件用意する。

Case A
Simple Blog

Case B
Relations + Media

Case C
Components + Dynamic Zones

それぞれをResource Definitionへ手動変換する。

検証：

表現できるか
表現しづらい概念は何か
MVP対象外として切れるか
将来のImporterに必要な情報は何か

---

0.3 Go + htmx UX Spike

特に以下を検証する。

Reference Picker
Validation errors
Publish action
JSON field

htmxだけで不自然な場合は、局所的JavaScriptを正式に採用する。

---

Exit Criteria

以下が動くこと。

ArticleをAdminから作成
ArticleをRESTから取得
cms validateでDBとのDriftを検出
Draft / Publishが動作

また、実在Strapi Schemaの主要なUnsupported領域を把握している。

---

Phase 1 — Core MVP

Goal

CMS Coreとして最低限必要な機能を完成させる。

---

1.1 Resource Definition

実装：

Resource
Field
Storage Mapping
Validation
Lifecycle
Reference

Field Type：

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

---

1.2 Database Introspection

PostgreSQL metadataを取得する。

Table
Column
Type
Nullable
Unique
Foreign key where useful

---

1.3 cms validate

CLI：

cms validate

CIでも利用可能にする。

Validation failure時は明確な修正メッセージを出す。

---

1.4 ContentService

最低限：

Get
List
Create
Update
Delete
Publish
Unpublish

HTTPには依存させない。

---

1.5 Repository

ContentRepository
PostgreSQL implementation

だけ。

複数Database対応はしない。

高度なCustom Repository APIは、必要以上に抽象化しない。

---

1.6 Lifecycle

none
managed

のみ。

managedの場合は明示的なcolumn mappingを要求する。

---

1.7 Audit

Mutation時に記録。

Create
Update
Delete
Publish
Unpublish

Content MutationとAuditを同一Transactionで保存する。

---

1.8 Revision

MVP最低限：

snapshot
version
schema_version
created_at
actor

Restoreは後回し可能。

---

Phase 2 — Useful Admin

Goal

開発者が日常的に使えるCMSにする。

---

2.1 Resource Navigation

Resource一覧
Content一覧

---

2.2 CRUD

Create
Edit
Delete

---

2.3 Lifecycle

Publish
Unpublish
Status display

---

2.4 Field Widgets

初期Widget：

Text
Textarea
Number
Checkbox
Datetime
Select
JSON
Reference Picker

---

2.5 Revision / Audit UI

最低限、

Revision一覧
Basic diff
Audit一覧

を表示する。

---

Admin Principle

Strapi完全再現を目指さない。

目標は、

«一般的なCRUDで説明なしに使える»

こと。

複雑なUI機能は追加しない。

---

Phase 3 — Headless API

Goal

Headless CMSとして実案件で利用可能にする。

---

3.1 REST

GET list
GET detail
POST
PATCH
DELETE

---

3.2 Query

最低限：

pagination
sort
basic filter

Reference includeは明示的な浅い機能に限定する。

---

3.3 OpenAPI 3.1

Resource Definitionから生成する。

cms openapi export

OpenAPI品質を重視する。

確認項目：

required
nullable
enum
request body
response schema
error schema
pagination

---

3.4 Client Generation Validation

CMS独自SDK Generatorは作らない。

既存Generatorで以下が問題なく生成できることを確認する。

TypeScript
Go

---

Milestone v0.1 — Useful CMS

v0.1のDefinition of Done：

Single Go binary

PostgreSQL

Resource Definition

cms validate

Admin CRUD

Draft / Publish

Audit

Basic Revision

REST API

OpenAPI 3.1

Reference

この時点で、実案件1件に導入する。

---

Phase 4 — Real-world Validation

Goal

機能追加より、実利用による設計検証を優先する。

最低1プロジェクトで利用する。

観察項目：

Resource Definitionが書きやすいか
DB migrationとの二重管理が苦痛か
Drift detectionで十分か
Admin UXが不足している箇所
Referenceの限界
Revisionの必要性
OpenAPI生成品質
Upgrade friction

ここでv0.2の優先順位を決める。

事前に全Phaseを固定しない。

---

Candidate v0.2 Features

実利用の結果から選ぶ。

優先候補：

Media
Revision Restore
RBAC
API Key
Webhooks
Strapi Importer
Native MCP
Many-to-Many Link Resource

すべて同時には実装しない。

---

Phase 5 — Strapi Migration

実利用で需要が確認できた場合に進める。

Schema Importer

cms import strapi

まず対応：

Collection Type
Single Type
Basic Field
Enum
Validation
Simple Reference

後から：

Media
Components
Dynamic Zones
Many-to-Many
i18n

---

Migration Report

出力：

AUTO
MANUAL REVIEW
UNSUPPORTED

100%互換を目標にしない。

目標：

«移行可能性を明確にし、人間の作業箇所を減らす。»

---

Phase 6 — Native Agent Interface

OpenAPI経由のAgent利用で不足が確認された場合に実装する。

First Native MCP Use Cases

Content CRUDを再実装しない。

まずControl Planeに集中する。

schema.get
schema.drift
audit.search
content.validate
revision.inspect
system.capabilities

---

Agent Roles

将来的に：

Content Agent
Coding Agent
SRE Agent
Security Agent
Data Quality Agent

を想定する。

Agent操作もAudit対象とする。

---

Phase 7 — Operations

需要に応じて追加する。

候補：

Webhooks
Scheduled Publish
Background Jobs
Outbox
OpenTelemetry
Health endpoints
Data consistency checks

Outboxは外部非同期配送が本当に必要になった時点で追加する。

---

Phase 8 — Distributed Runtime

明確なスケーリング要求が発生した場合のみ実装する。

MVPでは設計対象外。

将来的な候補：

admin-web
web-api
core-api
worker
mcp

ただし分散化を目的化しない。

分散化前に、

CDN
Read Replica
Horizontal scale of single binary

で解決できないかを先に検討する。

---

開発優先順位

迷った場合：

1. 実案件で使えるか
2. Upgrade safety
3. Schema ownership
4. Developer simplicity
5. Admin usability
6. OpenAPI quality
7. Extensibility
8. Agent features
9. Distributed runtime

---

YAGNIルール

以下は「必要になるかもしれない」だけでは作らない。

RemoteCoreClient
Microservices
Event Bus
Outbox
Plugin Marketplace
GraphQL
Multiple DB support
Native MCP CRUD
Workflow Engine
Visual Schema Builder

---

最初のGitHub Issues

#1  Repository skeleton
#2  Resource Definition model
#3  YAML parser
#4  PostgreSQL introspection
#5  cms validate
#6  ContentRepository
#7  ContentService
#8  Managed lifecycle
#9  Audit
#10 Basic Revision
#11 Article vertical slice
#12 Admin resource list
#13 Admin content list
#14 Admin create/edit form
#15 Reference picker
#16 Publish / Unpublish
#17 REST list/detail
#18 REST mutation
#19 OpenAPI generation
#20 cms init / cms serve

---

最初のマイルストーン

M0 — Schema Contract Works

Resource DefinitionとDBを検証できる。

M1 — Useful Admin

ArticleをAdminでCRUDできる。

M2 — Headless

RESTとOpenAPIが使える。

M3 — Generic CMS

Article以外のResourceをYAML追加だけで扱える。

M4 — Real-world

実案件で運用する。

M5 — Product Direction Validated

実利用からv0.2の優先順位を決定する。

---

初期KPI

Time to First Content

cms init
    ↓
DB Migration
    ↓
Resource Definition
    ↓
cms validate
    ↓
cms serve
    ↓
Create Content

10分以内を目標。

Time to New Resource

Resource追加からAdmin / REST / OpenAPI利用まで5〜10分以内。

Upgrade Safety

CMS minor upgradeに伴うContent Table Migration：

0

を基本とする。

Drift Detection

DBとResource Definitionの不整合をCIで検出可能。

Operational Simplicity

標準本番環境：

CMS binary
PostgreSQL

のみで成立する。

---

最終方針

MVPでは、

«Go製の、軽量で、予測可能で、DBを乗っ取らないHeadless CMS»

を完成させる。

その後、実利用から必要性が確認された機能だけを追加する。

Agent-native、Strapi Importer、分散Runtimeは重要な将来方向として維持するが、初期のCMS完成度より優先しない。


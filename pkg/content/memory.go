package content

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/yutapok/gohcms/pkg/schema"
)

// MemoryContentRepository provides an in-memory storage for content records segregated by resource table.
type MemoryContentRepository struct {
	mu      sync.RWMutex
	records map[string]map[string]*Record // table -> ID -> Record
}

// NewMemoryContentRepository creates a new in-memory content repository.
func NewMemoryContentRepository() *MemoryContentRepository {
	return &MemoryContentRepository{
		records: make(map[string]map[string]*Record),
	}
}

func (m *MemoryContentRepository) getTableMap(table string) map[string]*Record {
	if m.records[table] == nil {
		m.records[table] = make(map[string]*Record)
	}
	return m.records[table]
}

func (m *MemoryContentRepository) Get(ctx context.Context, def *schema.ResourceDefinition, id string) (*Record, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	tableMap, exists := m.records[def.Storage.Table]
	if !exists {
		return nil, fmt.Errorf("record '%s' not found in table '%s'", id, def.Storage.Table)
	}

	r, exists := tableMap[id]
	if !exists {
		return nil, fmt.Errorf("record '%s' not found in table '%s'", id, def.Storage.Table)
	}

	// Return a copy
	cp := *r
	cp.Data = make(map[string]interface{})
	for k, v := range r.Data {
		cp.Data[k] = v
	}
	return &cp, nil
}

func (m *MemoryContentRepository) List(ctx context.Context, def *schema.ResourceDefinition, filter ContentFilter, pagination Pagination) ([]*Record, int64, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	tableMap, exists := m.records[def.Storage.Table]
	if !exists {
		return []*Record{}, 0, nil
	}

	var list []*Record
	for _, r := range tableMap {
		if filter.Status != nil && r.Status != *filter.Status {
			continue
		}
		cp := *r
		cp.Data = make(map[string]interface{})
		for k, v := range r.Data {
			cp.Data[k] = v
		}
		list = append(list, &cp)
	}

	return list, int64(len(list)), nil
}

func (m *MemoryContentRepository) Create(ctx context.Context, def *schema.ResourceDefinition, record *Record) (*Record, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if record.ID == "" {
		record.ID = uuid.New().String()
	}
	if record.CreatedAt.IsZero() {
		record.CreatedAt = time.Now()
	}
	if record.UpdatedAt.IsZero() {
		record.UpdatedAt = time.Now()
	}

	tableMap := m.getTableMap(def.Storage.Table)
	tableMap[record.ID] = record
	return record, nil
}

func (m *MemoryContentRepository) Update(ctx context.Context, def *schema.ResourceDefinition, record *Record) (*Record, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	record.UpdatedAt = time.Now()
	tableMap := m.getTableMap(def.Storage.Table)
	tableMap[record.ID] = record
	return record, nil
}

func (m *MemoryContentRepository) Delete(ctx context.Context, def *schema.ResourceDefinition, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if tableMap, exists := m.records[def.Storage.Table]; exists {
		delete(tableMap, id)
	}
	return nil
}

// MemoryAuditRepository stores audit logs in memory.
type MemoryAuditRepository struct {
	mu   sync.RWMutex
	logs []*AuditLog
}

func NewMemoryAuditRepository() *MemoryAuditRepository {
	return &MemoryAuditRepository{}
}

func (m *MemoryAuditRepository) Insert(ctx context.Context, log *AuditLog) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if log.ID == "" {
		log.ID = uuid.New().String()
	}
	if log.Timestamp.IsZero() {
		log.Timestamp = time.Now()
	}
	m.logs = append(m.logs, log)
	return nil
}

func (m *MemoryAuditRepository) List(ctx context.Context, resource string, resourceID string) ([]*AuditLog, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var result []*AuditLog
	for _, l := range m.logs {
		if resource != "" && l.Resource != resource {
			continue
		}
		if resourceID != "" && l.ResourceID != resourceID {
			continue
		}
		result = append(result, l)
	}
	return result, nil
}

// MemoryRevisionRepository stores revisions in memory.
type MemoryRevisionRepository struct {
	mu   sync.RWMutex
	revs []*Revision
}

func NewMemoryRevisionRepository() *MemoryRevisionRepository {
	return &MemoryRevisionRepository{}
}

func (m *MemoryRevisionRepository) Insert(ctx context.Context, rev *Revision) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if rev.ID == "" {
		rev.ID = uuid.New().String()
	}
	if rev.CreatedAt.IsZero() {
		rev.CreatedAt = time.Now()
	}
	m.revs = append(m.revs, rev)
	return nil
}

func (m *MemoryRevisionRepository) Get(ctx context.Context, resource string, resourceID string, version int64) (*Revision, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	for _, r := range m.revs {
		if r.Resource == resource && r.ResourceID == resourceID && r.Version == version {
			return r, nil
		}
	}
	return nil, fmt.Errorf("revision not found")
}

func (m *MemoryRevisionRepository) List(ctx context.Context, resource string, resourceID string) ([]*Revision, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var result []*Revision
	for _, r := range m.revs {
		if resource != "" && r.Resource != resource {
			continue
		}
		if resourceID != "" && r.ResourceID != resourceID {
			continue
		}
		result = append(result, r)
	}
	return result, nil
}

// MemoryUnitOfWork coordinates in-memory or file-backed transaction callbacks.
type MemoryUnitOfWork struct {
	repo  ContentRepository
	audit *MemoryAuditRepository
	rev   *MemoryRevisionRepository
}

func NewMemoryUnitOfWork(repo ContentRepository, audit *MemoryAuditRepository, rev *MemoryRevisionRepository) *MemoryUnitOfWork {
	return &MemoryUnitOfWork{
		repo:  repo,
		audit: audit,
		rev:   rev,
	}
}

func (u *MemoryUnitOfWork) Execute(ctx context.Context, fn func(repo ContentRepository, auditRepo AuditRepository, revRepo RevisionRepository) error) error {
	return fn(u.repo, u.audit, u.rev)
}

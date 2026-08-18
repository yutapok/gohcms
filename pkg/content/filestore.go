package content

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/yutapok/gohcms/pkg/schema"
)

// DemoStateSnapshot holds the serialized state of content, audit logs, and revisions.
type DemoStateSnapshot struct {
	Records   map[string]map[string]*Record `json:"records"`
	AuditLogs []*AuditLog                   `json:"audit_logs"`
	Revisions []*Revision                   `json:"revisions"`
}

// FileBackedContentRepository provides a thread-safe repository that persists to and auto-reloads from a JSON file.
type FileBackedContentRepository struct {
	mu          sync.RWMutex
	filePath    string
	records     map[string]map[string]*Record
	lastModTime time.Time
}

// NewFileBackedContentRepository creates or loads a file-backed content repository.
func NewFileBackedContentRepository(filePath string) *FileBackedContentRepository {
	repo := &FileBackedContentRepository{
		filePath: filePath,
		records:  make(map[string]map[string]*Record),
	}
	repo.reloadIfModified()
	return repo
}

func (f *FileBackedContentRepository) reloadIfModified() {
	if f.filePath == "" {
		return
	}

	info, err := os.Stat(f.filePath)
	if err != nil {
		return
	}

	if info.ModTime().After(f.lastModTime) {
		data, err := os.ReadFile(f.filePath)
		if err == nil {
			var snap DemoStateSnapshot
			if err := json.Unmarshal(data, &snap); err == nil && snap.Records != nil {
				f.records = snap.Records
				f.lastModTime = info.ModTime()
			}
		}
	}
}

func (f *FileBackedContentRepository) saveToFileLocked() {
	if f.filePath == "" {
		return
	}

	snap := DemoStateSnapshot{
		Records: f.records,
	}

	data, err := json.MarshalIndent(snap, "", "  ")
	if err == nil {
		_ = os.WriteFile(f.filePath, data, 0644)
		if info, err := os.Stat(f.filePath); err == nil {
			f.lastModTime = info.ModTime()
		}
	}
}

func (f *FileBackedContentRepository) Get(ctx context.Context, def *schema.ResourceDefinition, id string) (*Record, error) {
	f.mu.Lock()
	f.reloadIfModified()
	f.mu.Unlock()

	f.mu.RLock()
	defer f.mu.RUnlock()

	tableMap, exists := f.records[def.Storage.Table]
	if !exists {
		return nil, fmt.Errorf("record '%s' not found in table '%s'", id, def.Storage.Table)
	}

	r, exists := tableMap[id]
	if !exists {
		return nil, fmt.Errorf("record '%s' not found in table '%s'", id, def.Storage.Table)
	}

	cp := *r
	cp.Data = make(map[string]interface{})
	for k, v := range r.Data {
		cp.Data[k] = v
	}
	return &cp, nil
}

func (f *FileBackedContentRepository) List(ctx context.Context, def *schema.ResourceDefinition, filter ContentFilter, pagination Pagination) ([]*Record, int64, error) {
	f.mu.Lock()
	f.reloadIfModified()
	f.mu.Unlock()

	f.mu.RLock()
	defer f.mu.RUnlock()

	tableMap, exists := f.records[def.Storage.Table]
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

func (f *FileBackedContentRepository) Create(ctx context.Context, def *schema.ResourceDefinition, record *Record) (*Record, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.reloadIfModified()

	if record.ID == "" {
		record.ID = uuid.New().String()
	}
	if record.CreatedAt.IsZero() {
		record.CreatedAt = time.Now()
	}
	if record.UpdatedAt.IsZero() {
		record.UpdatedAt = time.Now()
	}

	if f.records[def.Storage.Table] == nil {
		f.records[def.Storage.Table] = make(map[string]*Record)
	}
	f.records[def.Storage.Table][record.ID] = record

	f.saveToFileLocked()
	return record, nil
}

func (f *FileBackedContentRepository) Update(ctx context.Context, def *schema.ResourceDefinition, record *Record) (*Record, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.reloadIfModified()

	record.UpdatedAt = time.Now()
	if f.records[def.Storage.Table] == nil {
		f.records[def.Storage.Table] = make(map[string]*Record)
	}
	f.records[def.Storage.Table][record.ID] = record

	f.saveToFileLocked()
	return record, nil
}

func (f *FileBackedContentRepository) Delete(ctx context.Context, def *schema.ResourceDefinition, id string) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.reloadIfModified()

	if tableMap, exists := f.records[def.Storage.Table]; exists {
		delete(tableMap, id)
		f.saveToFileLocked()
	}
	return nil
}

var _ ContentRepository = (*FileBackedContentRepository)(nil)

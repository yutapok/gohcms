package job

import (
	"context"
	"fmt"

	"github.com/yutapok/gohcms/pkg/introspection"
	"github.com/yutapok/gohcms/pkg/schema"
	"github.com/yutapok/gohcms/pkg/validator"
)

// Job defines the interface for a one-shot CLI background or batch job.
type Job interface {
	Name() string
	Description() string
	Run(ctx context.Context, args []string) (int, error)
}

// Registry stores and executes available jobs.
type Registry struct {
	jobs map[string]Job
}

// NewRegistry creates a new Job Registry.
func NewRegistry() *Registry {
	return &Registry{
		jobs: make(map[string]Job),
	}
}

// Register registers a new job.
func (r *Registry) Register(j Job) {
	r.jobs[j.Name()] = j
}

// Get retrieves a job by name.
func (r *Registry) Get(name string) (Job, bool) {
	j, ok := r.jobs[name]
	return j, ok
}

// List returns all registered job names.
func (r *Registry) List() []string {
	var names []string
	for k := range r.jobs {
		names = append(names, k)
	}
	return names
}

// ValidateJob executes schema drift validation and returns exit status.
type ValidateJob struct {
	definitions []*schema.ResourceDefinition
	dbSchema    *introspection.DatabaseSchema
}

// NewValidateJob creates a new ValidateJob.
func NewValidateJob(definitions []*schema.ResourceDefinition, dbSchema *introspection.DatabaseSchema) *ValidateJob {
	return &ValidateJob{
		definitions: definitions,
		dbSchema:    dbSchema,
	}
}

func (j *ValidateJob) Name() string {
	return "validate"
}

func (j *ValidateJob) Description() string {
	return "Validate schema drift between Resource Definitions and Database Schema."
}

func (j *ValidateJob) Run(ctx context.Context, args []string) (int, error) {
	v := validator.New()
	result := v.ValidateAll(j.definitions, j.dbSchema)

	if result.IsValid() {
		fmt.Printf("✓ Schema validation successful: %d resource(s) valid.\n", len(j.definitions))
		return 0, nil
	}

	fmt.Println("⚠️  Schema Drift Detected:")
	fmt.Print(result.FormatReport())
	return 1, nil
}

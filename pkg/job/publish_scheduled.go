package job

import (
	"context"
	"fmt"
	"time"

	"github.com/yutapok/gohcms/pkg/content"
	"github.com/yutapok/gohcms/pkg/schema"
)

// PublishScheduledJob automatically publishes draft records whose published_at timestamp has passed.
type PublishScheduledJob struct {
	svc         *content.ContentService
	definitions []*schema.ResourceDefinition
}

// NewPublishScheduledJob creates a new PublishScheduledJob.
func NewPublishScheduledJob(svc *content.ContentService, definitions []*schema.ResourceDefinition) *PublishScheduledJob {
	return &PublishScheduledJob{
		svc:         svc,
		definitions: definitions,
	}
}

func (j *PublishScheduledJob) Name() string {
	return "publish-scheduled"
}

func (j *PublishScheduledJob) Description() string {
	return "Automatically publish draft records whose published_at date is in the past."
}

func (j *PublishScheduledJob) Run(ctx context.Context, args []string) (int, error) {
	now := time.Now()
	totalPublished := 0

	mctx := content.MutationContext{
		Actor:     "system",
		ActorType: content.ActorTypeSystem,
		RequestID: fmt.Sprintf("job-sched-%d", now.Unix()),
	}

	for _, def := range j.definitions {
		if def.Lifecycle.Mode != schema.LifecycleModeManaged {
			continue
		}

		// Check if resource has a published_at field
		hasPublishedAt := false
		var pubFieldName string
		for name, f := range def.Fields {
			if f.Type == schema.FieldTypeDateTime && (name == "published_at" || f.Column == "published_at") {
				hasPublishedAt = true
				pubFieldName = name
				break
			}
		}

		if !hasPublishedAt {
			continue
		}

		// Fetch all draft records
		draftStatus := content.StatusDraft
		records, _, err := j.svc.List(ctx, def, content.ContentFilter{Status: &draftStatus}, content.Pagination{Limit: 1000})
		if err != nil {
			fmt.Printf("⚠️  Failed to query draft records for %s: %v\n", def.Resource, err)
			continue
		}

		for _, rec := range records {
			val, exists := rec.GetField(pubFieldName)
			if !exists || val == nil {
				continue
			}

			pubTime, ok := parseDateTime(val)
			if !ok {
				continue
			}

			if pubTime.Before(now) || pubTime.Equal(now) {
				_, err := j.svc.Publish(ctx, def, rec.ID, mctx)
				if err != nil {
					fmt.Printf("⚠️  Failed to auto-publish %s (%s): %v\n", def.Resource, rec.ID, err)
				} else {
					fmt.Printf("✓ Auto-published %s '%s' (scheduled for: %s)\n", def.Resource, rec.ID, pubTime.Format(time.RFC3339))
					totalPublished++
				}
			}
		}
	}

	fmt.Printf("✓ Scheduled publishing batch completed: %d record(s) published.\n", totalPublished)
	return 0, nil
}

func parseDateTime(v interface{}) (time.Time, bool) {
	switch t := v.(type) {
	case time.Time:
		return t, true
	case string:
		if t == "" {
			return time.Time{}, false
		}
		// First try RFC3339 with explicit timezone offset
		if parsed, err := time.Parse(time.RFC3339, t); err == nil {
			return parsed, true
		}
		// Try formats using time.Local
		for _, layout := range []string{
			"2006-01-02T15:04:05",
			"2006-01-02T15:04",
			"2006-01-02 15:04:05",
			"2006-01-02 15:04",
			"2006-01-02",
		} {
			if parsed, err := time.ParseInLocation(layout, t, time.Local); err == nil {
				return parsed, true
			}
		}
	}
	return time.Time{}, false
}

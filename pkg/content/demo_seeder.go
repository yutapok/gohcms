package content

import (
	"context"
	"time"

	"github.com/yutapok/gohcms/pkg/schema"
)

// SeedDemoData seeds initial rich demonstration articles with dependencies, lifecycle states, and scheduled publish dates.
func SeedDemoData(ctx context.Context, svc *ContentService, definitions []*schema.ResourceDefinition) {
	mctx := MutationContext{Actor: "demo-admin", ActorType: ActorTypeUser, RequestID: "req-demo-init"}

	for _, def := range definitions {
		if def.Resource != "article" {
			continue
		}

		// Check if records already exist
		existing, _, _ := svc.List(ctx, def, ContentFilter{}, Pagination{Limit: 10})
		if len(existing) > 0 {
			return // Already seeded
		}

		// 1. Published Chapter 1
		c1, _ := svc.Create(ctx, def, map[string]interface{}{
			"id":    "art-1",
			"title": "Chapter 1: The Agent-Native CMS Architecture",
			"body":  "Headless CMS built in pure Go with server-driven htmx and explicit storage conventions.",
		}, mctx)
		svc.Publish(ctx, def, c1.ID, mctx)

		// 2. Published Chapter 2 (depends on 1)
		c2, _ := svc.Create(ctx, def, map[string]interface{}{
			"id":         "art-2",
			"title":      "Chapter 2: Kanban & Interactive Lifecycle Transitions",
			"body":       "Drag-and-drop cards between Draft, Published, and Finished states seamlessly.",
			"depends_on": "art-1",
		}, mctx)
		svc.Publish(ctx, def, c2.ID, mctx)

		// 3. Draft Chapter 3 (depends on 2)
		svc.Create(ctx, def, map[string]interface{}{
			"id":         "art-3",
			"title":      "Chapter 3: Headless REST & OpenAPI 3.1 Contract",
			"body":       "Upcoming article currently in draft stage waiting for review.",
			"depends_on": "art-2",
		}, mctx)

		// 4. Draft with Past Published_At (Ready for cms job publish-scheduled)
		pastPubTime := time.Now().Add(-2 * time.Hour).Format(time.RFC3339)
		svc.Create(ctx, def, map[string]interface{}{
			"id":           "art-sched",
			"title":        "⚡ Scheduled Launch Announcement (Auto-Publish Target)",
			"body":         "This draft article is scheduled in the past and will be auto-published by the job runner.",
			"published_at": pastPubTime,
		}, mctx)

		// 5. Finished Archive Article
		c5, _ := svc.Create(ctx, def, map[string]interface{}{
			"id":    "art-old",
			"title": "Archive: Initial 2024 Product Whitepaper",
			"body":  "Past campaign documentation archived for audit and reference purposes.",
		}, mctx)
		svc.Publish(ctx, def, c5.ID, mctx)
		svc.Finish(ctx, def, c5.ID, mctx)
	}
}

package content

import (
	"fmt"
)

// ContentStatus represents the lifecycle state of a content record.
type ContentStatus string

const (
	StatusDraft     ContentStatus = "draft"
	StatusPublished ContentStatus = "published"
	StatusFinished  ContentStatus = "finished"
)

// IsValidStatus returns true if the status string is recognized.
func IsValidStatus(s string) bool {
	switch ContentStatus(s) {
	case StatusDraft, StatusPublished, StatusFinished:
		return true
	default:
		return false
	}
}

// CanTransition validates whether a status transition is permitted.
func CanTransition(from, to ContentStatus) error {
	if from == to {
		return nil
	}

	switch from {
	case StatusDraft:
		if to == StatusPublished {
			return nil
		}
	case StatusPublished:
		if to == StatusDraft || to == StatusFinished {
			return nil
		}
	case StatusFinished:
		if to == StatusDraft {
			return nil // Re-opening finished content back to draft for editing
		}
	}

	return fmt.Errorf("invalid status transition from '%s' to '%s'", from, to)
}

package validator

import (
	"fmt"
	"strings"
)

// ValidationError represents an issue found during validation.
type ValidationError struct {
	Resource string
	Table    string
	Field    string
	Column   string
	Message  string
}

func (e ValidationError) Error() string {
	if e.Field != "" && e.Column != "" {
		return fmt.Sprintf("resource '%s' (table '%s', field '%s' -> column '%s'): %s", e.Resource, e.Table, e.Field, e.Column, e.Message)
	}
	if e.Column != "" {
		return fmt.Sprintf("resource '%s' (table '%s', column '%s'): %s", e.Resource, e.Table, e.Column, e.Message)
	}
	return fmt.Sprintf("resource '%s' (table '%s'): %s", e.Resource, e.Table, e.Message)
}

// ValidationPassed represents a successfully validated item.
type ValidationPassed struct {
	Resource string
	Table    string
	Field    string
	Column   string
	Details  string
}

// ValidationResult contains all validation successes and errors.
type ValidationResult struct {
	PassedItems []ValidationPassed
	Errors      []ValidationError
}

// IsValid returns true if there are no validation errors.
func (r *ValidationResult) IsValid() bool {
	return len(r.Errors) == 0
}

// AddError adds a new ValidationError.
func (r *ValidationResult) AddError(err ValidationError) {
	r.Errors = append(r.Errors, err)
}

// AddPassed adds a new ValidationPassed item.
func (r *ValidationResult) AddPassed(item ValidationPassed) {
	r.PassedItems = append(r.PassedItems, item)
}

// Merge merges another ValidationResult into this one.
func (r *ValidationResult) Merge(other *ValidationResult) {
	if other == nil {
		return
	}
	r.PassedItems = append(r.PassedItems, other.PassedItems...)
	r.Errors = append(r.Errors, other.Errors...)
}

// FormatReport generates a human-readable CLI validation report.
func (r *ValidationResult) FormatReport() string {
	var sb strings.Builder

	if len(r.PassedItems) > 0 {
		for _, item := range r.PassedItems {
			if item.Field != "" {
				sb.WriteString(fmt.Sprintf("✓ %s.%s -> %s.%s (%s)\n", item.Resource, item.Field, item.Table, item.Column, item.Details))
			} else {
				sb.WriteString(fmt.Sprintf("✓ %s (%s)\n", item.Resource, item.Details))
			}
		}
	}

	if len(r.Errors) > 0 {
		if sb.Len() > 0 {
			sb.WriteString("\n")
		}
		sb.WriteString("ERROR:\n")
		for _, err := range r.Errors {
			sb.WriteString(fmt.Sprintf("  - %s\n", err.Error()))
		}
	}

	return sb.String()
}

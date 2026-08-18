package media

import (
	"fmt"
	"time"
)

// Media represents an uploaded file metadata record.
type Media struct {
	ID        string    `json:"id"`
	Filename  string    `json:"filename"`
	Filepath  string    `json:"-"` // Internal storage path, hidden from API responses
	MimeType  string    `json:"mime_type"`
	SizeBytes int64     `json:"size_bytes"`
	CreatedAt time.Time `json:"created_at"`
	URL       string    `json:"url"` // Dynamic URL e.g. /media/123
}

// FormatURL populates the public URL field.
func (m *Media) FormatURL() {
	m.URL = fmt.Sprintf("/media/%s", m.ID)
}

// IsImage returns true if the MIME type represents a web-viewable image.
func (m *Media) IsImage() bool {
	return m.MimeType == "image/jpeg" || m.MimeType == "image/png" || m.MimeType == "image/webp" || m.MimeType == "image/gif" || m.MimeType == "image/svg+xml"
}

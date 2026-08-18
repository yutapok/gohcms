package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// Permission defines API Key access level.
type Permission string

const (
	PermissionRead      Permission = "read"
	PermissionReadWrite Permission = "read_write"
)

// APIKey represents an API token metadata and hash in the database.
type APIKey struct {
	ID         string     `json:"id"`
	Name       string     `json:"name"`
	TokenHash  string     `json:"token_hash"`
	Permission Permission `json:"permission"`
	IsActive   bool       `json:"is_active"`
	CreatedAt  time.Time  `json:"created_at"`
	RevokedAt  *time.Time `json:"revoked_at,omitempty"`
}

// CanAccessMethod checks if the API key has permission for the HTTP method.
func (k *APIKey) CanAccessMethod(method string) bool {
	if !k.IsActive {
		return false
	}
	if k.Permission == PermissionReadWrite {
		return true
	}
	if k.Permission == PermissionRead {
		return method == "GET" || method == "HEAD" || method == "OPTIONS"
	}
	return false
}

// HashToken computes SHA-256 hex string of the given raw token.
func HashToken(token string) string {
	hash := sha256.Sum256([]byte(token))
	return hex.EncodeToString(hash[:])
}

// GenerateToken creates a new secure random token with prefix and returns raw token, hash, and model.
func GenerateToken(name string, perm Permission) (rawToken string, key *APIKey, err error) {
	bytes := make([]byte, 24)
	if _, err := rand.Read(bytes); err != nil {
		return "", nil, fmt.Errorf("failed to generate random bytes: %w", err)
	}

	rawToken = "gohcms_live_" + hex.EncodeToString(bytes)
	tokenHash := HashToken(rawToken)

	key = &APIKey{
		ID:         uuid.New().String(),
		Name:       name,
		TokenHash:  tokenHash,
		Permission: perm,
		IsActive:   true,
		CreatedAt:  time.Now(),
	}

	return rawToken, key, nil
}

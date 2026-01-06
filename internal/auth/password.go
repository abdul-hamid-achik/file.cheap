package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"

	"golang.org/x/crypto/bcrypt"
)

const (
	// bcrypt cost factor - higher is more secure but slower
	bcryptCost = 12
	// Token length in bytes (before base64 encoding)
	tokenLength = 32
)

// HashPassword hashes a password using bcrypt.
func HashPassword(password string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcryptCost)
	if err != nil {
		return "", fmt.Errorf("failed to hash password: %w", err)
	}
	return string(hash), nil
}

// CheckPassword compares a password with a bcrypt hash.
// Returns nil if the password matches, error otherwise.
func CheckPassword(password, hash string) error {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
}

// GenerateToken generates a cryptographically secure random token.
// Returns the raw token (for sending to user) and its hash (for storage).
func GenerateToken() (token string, hash string, err error) {
	// Generate random bytes
	bytes := make([]byte, tokenLength)
	if _, err := rand.Read(bytes); err != nil {
		return "", "", fmt.Errorf("failed to generate random token: %w", err)
	}

	// Encode as URL-safe base64 (no padding)
	token = base64.RawURLEncoding.EncodeToString(bytes)

	// Hash the token for storage
	hash = HashToken(token)

	return token, hash, nil
}

// HashToken creates a SHA-256 hash of a token for storage.
// This is used for session tokens, password reset tokens, etc.
func HashToken(token string) string {
	hash := sha256.Sum256([]byte(token))
	return base64.RawURLEncoding.EncodeToString(hash[:])
}

// ValidatePassword checks if a password meets minimum requirements.
func ValidatePassword(password string) error {
	if len(password) < 8 {
		return fmt.Errorf("password must be at least 8 characters")
	}
	if len(password) > 128 {
		return fmt.Errorf("password must be at most 128 characters")
	}
	return nil
}

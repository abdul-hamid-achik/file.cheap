package cloudauth

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/abdul-hamid-achik/file.cheap/internal/fcheap/config"
)

type Credentials struct {
	ServiceURL          string    `json:"service_url"`
	Token               string    `json:"token"`
	RefreshToken        string    `json:"refresh_token,omitempty"`
	AccessExpiresAt     time.Time `json:"access_expires_at,omitempty"`
	RefreshExpiresAt    time.Time `json:"refresh_expires_at,omitempty"`
	PendingRefreshToken string    `json:"pending_refresh_token,omitempty"`
	PendingRotationID   string    `json:"pending_rotation_id,omitempty"`
}

func Path() (string, error) {
	dir, err := config.Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "credentials.json"), nil
}

func Load() (Credentials, error) {
	path, err := Path()
	if err != nil {
		return Credentials{}, err
	}
	info, err := os.Lstat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return Credentials{}, errors.New("not logged in; run fcheap auth login")
		}
		return Credentials{}, fmt.Errorf("inspect credentials: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return Credentials{}, errors.New("credentials path must be a regular file, not a symlink")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return Credentials{}, fmt.Errorf("read credentials: %w", err)
	}
	var value Credentials
	if err := json.Unmarshal(data, &value); err != nil {
		return Credentials{}, fmt.Errorf("decode credentials: %w", err)
	}
	if value.ServiceURL == "" || value.Token == "" {
		return Credentials{}, errors.New("stored credentials are incomplete; run fcheap auth login again")
	}
	if err := validateCredentials(value); err != nil {
		return Credentials{}, err
	}
	return value, nil
}

func Save(value Credentials) error {
	path, err := Path()
	if err != nil {
		return err
	}
	if err := validateCredentials(value); err != nil {
		return err
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("create credentials directory: %w", err)
	}
	if info, statErr := os.Lstat(path); statErr == nil && info.Mode()&os.ModeSymlink != 0 {
		return errors.New("refusing to replace a symlink at the credentials path")
	} else if statErr != nil && !os.IsNotExist(statErr) {
		return fmt.Errorf("inspect credentials: %w", statErr)
	}
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return fmt.Errorf("encode credentials: %w", err)
	}
	temporary, err := os.CreateTemp(dir, ".credentials-*.tmp")
	if err != nil {
		return fmt.Errorf("create temporary credentials: %w", err)
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath) //nolint:errcheck
	if err := temporary.Chmod(0600); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("secure temporary credentials: %w", err)
	}
	if _, err := temporary.Write(append(data, '\n')); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("write credentials: %w", err)
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("sync credentials: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close credentials: %w", err)
	}
	if err := os.Rename(temporaryPath, path); err != nil {
		return fmt.Errorf("install credentials: %w", err)
	}
	return os.Chmod(path, 0600)
}

func FromToken(serviceURL string, token Token, now time.Time) (Credentials, error) {
	if token.ExpiresIn <= 0 || token.RefreshExpiresIn <= 0 {
		return Credentials{}, errors.New("authentication response contained invalid token lifetimes")
	}
	value := Credentials{
		AccessExpiresAt:  now.Add(time.Duration(token.ExpiresIn) * time.Second),
		RefreshExpiresAt: now.Add(time.Duration(token.RefreshExpiresIn) * time.Second),
		RefreshToken:     token.RefreshToken,
		ServiceURL:       serviceURL,
		Token:            token.AccessToken,
	}
	if err := validateCredentials(value); err != nil {
		return Credentials{}, err
	}
	return value, nil
}

// BeginRefresh durably defines the replacement secret before it is sent. If a
// previous attempt lost its response, calling it again returns the same pair so
// the server can replay that rotation idempotently.
func BeginRefresh(value Credentials) (Credentials, error) {
	if !validRefreshToken(value.RefreshToken) {
		return Credentials{}, errors.New("stored credentials cannot be refreshed; run fcheap auth login again")
	}
	if value.PendingRefreshToken != "" || value.PendingRotationID != "" {
		if !validRefreshToken(value.PendingRefreshToken) || !validRotationID(value.PendingRotationID) {
			return Credentials{}, errors.New("stored refresh rotation is incomplete; run fcheap auth login again")
		}
		return value, nil
	}
	secret, err := randomBase64URL(32)
	if err != nil {
		return Credentials{}, fmt.Errorf("generate replacement refresh token: %w", err)
	}
	rotationID, err := randomBase64URL(16)
	if err != nil {
		return Credentials{}, fmt.Errorf("generate refresh rotation id: %w", err)
	}
	value.PendingRefreshToken = "fcheap_refresh_" + secret
	value.PendingRotationID = rotationID
	return value, nil
}

func CompleteRefresh(value Credentials, token Token, now time.Time) (Credentials, error) {
	if value.PendingRefreshToken == "" || token.RefreshToken != value.PendingRefreshToken {
		return Credentials{}, errors.New("refresh response did not match the pending replacement token")
	}
	if token.ExpiresIn <= 0 || token.RefreshExpiresIn <= 0 {
		return Credentials{}, errors.New("refresh response contained invalid token lifetimes")
	}
	value.Token = token.AccessToken
	value.RefreshToken = token.RefreshToken
	value.AccessExpiresAt = now.Add(time.Duration(token.ExpiresIn) * time.Second)
	value.RefreshExpiresAt = now.Add(time.Duration(token.RefreshExpiresIn) * time.Second)
	value.PendingRefreshToken = ""
	value.PendingRotationID = ""
	if err := validateCredentials(value); err != nil {
		return Credentials{}, err
	}
	return value, nil
}

func validateCredentials(value Credentials) error {
	if _, err := validatedOrigin(value.ServiceURL); err != nil {
		return err
	}
	if !validAccessToken(value.Token) {
		return errors.New("refusing to store an invalid console access token")
	}
	if value.RefreshToken != "" && !validRefreshToken(value.RefreshToken) {
		return errors.New("refusing to store an invalid console refresh token")
	}
	if (value.PendingRefreshToken == "") != (value.PendingRotationID == "") {
		return errors.New("refusing to store an incomplete refresh rotation")
	}
	if value.PendingRefreshToken != "" && (!validRefreshToken(value.PendingRefreshToken) || !validRotationID(value.PendingRotationID)) {
		return errors.New("refusing to store an invalid refresh rotation")
	}
	return nil
}

func validAccessToken(value string) bool {
	return strings.HasPrefix(value, "fcheap_device_") && validBase64URL(strings.TrimPrefix(value, "fcheap_device_"), 43)
}

func validRefreshToken(value string) bool {
	return strings.HasPrefix(value, "fcheap_refresh_") && validBase64URL(strings.TrimPrefix(value, "fcheap_refresh_"), 43)
}

func validRotationID(value string) bool {
	return validBase64URL(value, len(value)) && len(value) >= 22 && len(value) <= 64
}

func validBase64URL(value string, length int) bool {
	if len(value) != length {
		return false
	}
	for _, character := range value {
		if !validBase64URLCharacter(character) {
			return false
		}
	}
	return true
}

func validBase64URLCharacter(character rune) bool {
	return (character >= 'A' && character <= 'Z') ||
		(character >= 'a' && character <= 'z') ||
		(character >= '0' && character <= '9') ||
		character == '_' || character == '-'
}

func randomBase64URL(size int) (string, error) {
	buffer := make([]byte, size)
	if _, err := rand.Read(buffer); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buffer), nil
}

func Drop() error {
	path, err := Path()
	if err != nil {
		return err
	}
	info, err := os.Lstat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("inspect credentials: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return errors.New("credentials path must be a regular file, not a symlink")
	}
	if err := os.Remove(path); err != nil {
		return fmt.Errorf("remove credentials: %w", err)
	}
	return nil
}

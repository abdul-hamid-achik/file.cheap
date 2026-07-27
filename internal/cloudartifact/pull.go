// Package cloudartifact restores one verified object from file.cheap's private
// artifact service. Control requests use the paired device credential; large
// bytes move directly from the short-lived signed transfer URL.
package cloudartifact

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/abdul-hamid-achik/file.cheap/internal/artifactref"
)

const (
	// MaxBytes mirrors the private platform's global artifact ceiling.
	MaxBytes                  int64 = 64 * 1024 * 1024
	maxControlBody                  = 64 * 1024
	controlTimeout                  = 15 * time.Second
	transferBaseTimeout             = 60 * time.Second
	minTransferBytesPerSecond int64 = 256 * 1024
	maxTransferTimeout              = 15 * time.Minute
)

var (
	artifactIDPattern  = regexp.MustCompile(`^art_[A-Za-z0-9_-]{16,96}$`)
	deviceTokenPattern = regexp.MustCompile(
		`^fcheap_device_[A-Za-z0-9_-]{43}$`,
	)
	headerNamePattern = regexp.MustCompile(`^[A-Za-z0-9-]{1,128}$`)
	sha256Pattern     = regexp.MustCompile(`^[a-f0-9]{64}$`)
)

// Options binds one owner-scoped artifact to an explicit destination.
type Options struct {
	ArtifactID  string
	Destination string
	ServiceURL  string
	Token       string
}

// Result describes verified local bytes without retaining a signed URL.
type Result struct {
	Version      string                    `json:"version"`
	ArtifactRef  artifactref.ArtifactRefV1 `json:"artifact_ref"`
	OutputPath   string                    `json:"output_path"`
	SHA256       string                    `json:"sha256"`
	SizeBytes    int64                     `json:"size_bytes"`
	Verification string                    `json:"verification"`
}

// Client performs the owner control request and direct verified transfer.
type Client struct {
	httpClient *http.Client
}

// NewClient constructs a pull client. Redirects are rejected even when the
// supplied client would normally follow them, so signed query parameters and
// grant headers cannot cross origins.
func NewClient(httpClient *http.Client) *Client {
	if httpClient == nil {
		httpClient = &http.Client{}
	}
	copy := *httpClient
	copy.CheckRedirect = func(_ *http.Request, _ []*http.Request) error {
		return http.ErrUseLastResponse
	}
	return &Client{httpClient: &copy}
}

// Pull requests a one-minute owner grant, streams at most the declared bytes,
// verifies size and SHA-256, then atomically links the verified temporary file
// into a destination that must not already exist.
func (c *Client) Pull(ctx context.Context, opts Options) (Result, error) {
	origin, destination, err := validateOptions(opts)
	if err != nil {
		return Result{}, err
	}

	grant, err := c.requestGrant(ctx, origin, opts.ArtifactID, opts.Token)
	if err != nil {
		return Result{}, err
	}
	if err := validateGrant(grant, opts.ArtifactID); err != nil {
		return Result{}, err
	}
	if err := c.download(ctx, grant, destination); err != nil {
		return Result{}, err
	}

	return Result{
		Version:      "filecheap-pull/1",
		ArtifactRef:  grant.ArtifactRef,
		OutputPath:   destination,
		SHA256:       grant.Artifact.SHA256,
		SizeBytes:    grant.Artifact.SizeBytes,
		Verification: grant.Artifact.Verification,
	}, nil
}

type artifactDetails struct {
	ArtifactID   string               `json:"artifactId"`
	CommittedAt  *string              `json:"committedAt"`
	ContentType  string               `json:"contentType"`
	ExpiresAt    *string              `json:"expiresAt"`
	Kind         string               `json:"kind"`
	Producer     artifactref.Producer `json:"producer"`
	SHA256       string               `json:"sha256"`
	SizeBytes    int64                `json:"sizeBytes"`
	State        string               `json:"state"`
	Verification string               `json:"verification"`
}

type transferGrant struct {
	ExpiresAt string            `json:"expiresAt"`
	Headers   map[string]string `json:"headers"`
	Method    string            `json:"method"`
	URL       string            `json:"url"`
}

type serviceResponse struct {
	Artifact    artifactDetails           `json:"artifact"`
	ArtifactRef artifactref.ArtifactRefV1 `json:"artifactRef"`
	Download    transferGrant             `json:"download"`
}

func (c *Client) requestGrant(
	ctx context.Context,
	origin string,
	artifactID string,
	token string,
) (serviceResponse, error) {
	body, err := json.Marshal(map[string]string{"artifactId": artifactID})
	if err != nil {
		return serviceResponse{}, fmt.Errorf("encode artifact download request: %w", err)
	}
	requestContext, cancel := context.WithTimeout(ctx, controlTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(
		requestContext,
		http.MethodPost,
		origin+"/api/console/artifacts/downloads",
		bytes.NewReader(body),
	)
	if err != nil {
		return serviceResponse{}, fmt.Errorf("create artifact download request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	response, err := c.httpClient.Do(req)
	if err != nil {
		return serviceResponse{}, errors.New("request artifact download grant failed")
	}
	defer response.Body.Close() //nolint:errcheck
	if response.StatusCode != http.StatusCreated {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, maxControlBody+1))
		return serviceResponse{}, fmt.Errorf(
			"artifact download grant returned unexpected status %d",
			response.StatusCode,
		)
	}
	decoder := json.NewDecoder(io.LimitReader(response.Body, maxControlBody+1))
	decoder.DisallowUnknownFields()
	var value serviceResponse
	if err := decoder.Decode(&value); err != nil {
		return serviceResponse{}, fmt.Errorf("decode artifact download grant: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return serviceResponse{}, errors.New("artifact download grant contains trailing JSON")
		}
		return serviceResponse{}, fmt.Errorf("decode artifact download grant: %w", err)
	}
	return value, nil
}

func (c *Client) download(
	ctx context.Context,
	grant serviceResponse,
	destination string,
) error {
	requestContext, cancel := context.WithTimeout(
		ctx,
		transferTimeoutFor(grant.Artifact.SizeBytes),
	)
	defer cancel()
	req, err := http.NewRequestWithContext(
		requestContext,
		http.MethodGet,
		grant.Download.URL,
		nil,
	)
	if err != nil {
		return errors.New("create direct artifact download request failed")
	}
	for name, value := range grant.Download.Headers {
		if !safeGrantHeader(name, value) {
			return errors.New("artifact service returned an unsafe download header")
		}
		req.Header.Set(name, value)
	}
	response, err := c.httpClient.Do(req)
	if err != nil {
		// A net/url error embeds the signed URL. Keep it out of user-visible and
		// loggable error chains.
		return errors.New("direct artifact download failed")
	}
	defer response.Body.Close() //nolint:errcheck
	if response.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 8*1024))
		return fmt.Errorf(
			"direct artifact download returned unexpected status %d",
			response.StatusCode,
		)
	}
	if response.ContentLength >= 0 && response.ContentLength != grant.Artifact.SizeBytes {
		return errors.New("downloaded artifact size does not match verified metadata")
	}

	temporary, err := os.CreateTemp(filepath.Dir(destination), ".fcheap-pull-*")
	if err != nil {
		return fmt.Errorf("create verified download staging file: %w", err)
	}
	temporaryPath := temporary.Name()
	installed := false
	defer func() {
		_ = temporary.Close()
		if !installed {
			_ = os.Remove(temporaryPath)
		}
	}()
	if err := temporary.Chmod(0o600); err != nil {
		return fmt.Errorf("secure verified download staging file: %w", err)
	}

	digest := sha256.New()
	written, err := io.Copy(
		io.MultiWriter(temporary, digest),
		io.LimitReader(response.Body, grant.Artifact.SizeBytes+1),
	)
	if err != nil {
		return errors.New("stream direct artifact download failed")
	}
	if written != grant.Artifact.SizeBytes ||
		hex.EncodeToString(digest.Sum(nil)) != grant.Artifact.SHA256 {
		return errors.New("downloaded artifact failed SHA-256 verification")
	}
	if err := temporary.Sync(); err != nil {
		return fmt.Errorf("sync verified download staging file: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close verified download staging file: %w", err)
	}
	if err := os.Link(temporaryPath, destination); err != nil {
		if errors.Is(err, os.ErrExist) {
			return fmt.Errorf("pull destination already exists: %s", destination)
		}
		return fmt.Errorf("install verified artifact at destination: %w", err)
	}
	installed = true
	if err := os.Remove(temporaryPath); err != nil {
		return fmt.Errorf("remove verified download staging file: %w", err)
	}
	return nil
}

func validateOptions(opts Options) (string, string, error) {
	if !artifactIDPattern.MatchString(opts.ArtifactID) {
		return "", "", errors.New("pull artifact ID is invalid")
	}
	if !deviceTokenPattern.MatchString(opts.Token) {
		return "", "", errors.New("a valid paired device credential is required")
	}
	origin, err := validatedOrigin(opts.ServiceURL)
	if err != nil {
		return "", "", err
	}
	if strings.TrimSpace(opts.Destination) == "" {
		return "", "", errors.New("pull destination is required")
	}
	destination, err := filepath.Abs(opts.Destination)
	if err != nil {
		return "", "", fmt.Errorf("resolve pull destination: %w", err)
	}
	parent, err := os.Stat(filepath.Dir(destination))
	if err != nil || !parent.IsDir() {
		return "", "", errors.New("pull destination parent must be an existing directory")
	}
	if _, err := os.Lstat(destination); err == nil {
		return "", "", fmt.Errorf("pull destination already exists: %s", destination)
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", "", fmt.Errorf("inspect pull destination: %w", err)
	}
	return origin, destination, nil
}

func validateGrant(value serviceResponse, artifactID string) error {
	artifact := value.Artifact
	if artifact.ArtifactID != artifactID || artifact.State != "committed" ||
		artifact.CommittedAt == nil || artifact.Verification != "server-sha256" ||
		!sha256Pattern.MatchString(artifact.SHA256) || artifact.SizeBytes <= 0 ||
		artifact.SizeBytes > MaxBytes {
		return errors.New("artifact service returned inconsistent verified metadata")
	}
	if err := value.ArtifactRef.Validate(); err != nil {
		return fmt.Errorf("validate artifact download reference: %w", err)
	}
	if value.ArtifactRef.Provider != artifactref.ProviderCloud ||
		value.ArtifactRef.ArtifactID != artifactID ||
		value.ArtifactRef.Kind != artifact.Kind ||
		value.ArtifactRef.URI != "fcheap://cloud/vaults/private/artifacts/"+artifactID ||
		value.ArtifactRef.Producer == nil ||
		*value.ArtifactRef.Producer != artifact.Producer {
		return errors.New("artifact service returned a mismatched artifact reference")
	}
	if value.Download.Method != http.MethodGet || value.Download.URL == "" {
		return errors.New("artifact service returned an invalid download grant")
	}
	if _, err := time.Parse(time.RFC3339, value.Download.ExpiresAt); err != nil {
		return errors.New("artifact service returned an invalid download expiry")
	}
	parsed, err := url.Parse(value.Download.URL)
	if err != nil || !safeTransferURL(parsed) {
		return errors.New("artifact service returned an unsafe download URL")
	}
	for name, headerValue := range value.Download.Headers {
		if !safeGrantHeader(name, headerValue) {
			return errors.New("artifact service returned an unsafe download header")
		}
	}
	return nil
}

func validatedOrigin(raw string) (string, error) {
	value := strings.TrimSpace(raw)
	parsed, err := url.Parse(value)
	if err != nil || parsed.Host == "" || parsed.User != nil ||
		parsed.RawQuery != "" || parsed.Fragment != "" ||
		(parsed.Path != "" && parsed.Path != "/") {
		return "", errors.New("artifact service URL must be a bare HTTPS origin, or HTTP loopback for local development")
	}
	loopback := isLoopback(parsed.Hostname())
	if parsed.Scheme != "https" && (parsed.Scheme != "http" || !loopback) {
		return "", errors.New("artifact service URL must use HTTPS outside loopback")
	}
	return strings.TrimSuffix(parsed.String(), "/"), nil
}

func safeTransferURL(value *url.URL) bool {
	if value == nil || value.Host == "" || value.User != nil ||
		value.Fragment != "" || len(value.String()) > 16_384 {
		return false
	}
	return value.Scheme == "https" ||
		(value.Scheme == "http" && isLoopback(value.Hostname()))
}

func safeGrantHeader(name, value string) bool {
	lower := strings.ToLower(name)
	if !headerNamePattern.MatchString(name) || value == "" || len(value) > 8*1024 ||
		strings.ContainsAny(value, "\r\n") {
		return false
	}
	switch lower {
	case "authorization", "cookie", "host", "proxy-authorization", "x-api-key":
		return false
	default:
		return true
	}
}

func isLoopback(host string) bool {
	return host == "127.0.0.1" || host == "localhost" || host == "::1"
}

func transferTimeoutFor(size int64) time.Duration {
	timeout := transferBaseTimeout +
		time.Duration(size/minTransferBytesPerSecond)*time.Second
	if timeout > maxTransferTimeout {
		return maxTransferTimeout
	}
	return timeout
}

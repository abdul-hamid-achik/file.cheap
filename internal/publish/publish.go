// Package publish uploads one bounded local artifact through file.cheap's
// private artifact service. It never manages Vercel, Blob, or database
// credentials and never removes the local source.
package publish

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"hash"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"regexp"
	"strings"
	"time"

	"github.com/abdul-hamid-achik/file.cheap/internal/artifactref"
)

const (
	// MaxBytes is the global file.cheap artifact ceiling. The service enforces
	// the same value plus a smaller per-producer quota, so a publication can
	// still be rejected with 413 below this bound.
	MaxBytes        int64 = 64 * 1024 * 1024
	maxControlBody        = 64 * 1024
	maxTransferBody       = 8 * 1024
	controlTimeout        = 15 * time.Second
	// The direct PUT deadline scales with the artifact so a large upload on a
	// slow uplink is not cut off, while staying bounded.
	transferBaseTimeout       = 60 * time.Second
	minTransferBytesPerSecond = 256 * 1024
	maxTransferTimeout        = 15 * time.Minute
	hashBufferBytes           = 256 * 1024
	minRetention              = time.Minute
	maxRetention              = 31 * 24 * time.Hour
)

var publisherTokenPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{43,128}$`)

type Options struct {
	ContentType string
	ExpiresIn   time.Duration
	Kind        string
	Producer    artifactref.Producer
	ServiceURL  string
	Token       string
}

type Receipt struct {
	Version      string                    `json:"version"`
	ArtifactRef  artifactref.ArtifactRefV1 `json:"artifact_ref"`
	SHA256       string                    `json:"sha256"`
	SizeBytes    int64                     `json:"size_bytes"`
	Verification string                    `json:"verification"`
	PublishedAt  string                    `json:"published_at"`
}

type Client struct {
	httpClient *http.Client
	now        func() time.Time
}

func NewClient(httpClient *http.Client) *Client {
	if httpClient == nil {
		httpClient = &http.Client{}
	}
	return &Client{httpClient: httpClient, now: time.Now}
}

// Publish streams a regular file once to hash it, then runs
// plan -> direct PUT -> commit while streaming the same bytes again. Memory
// stays constant regardless of artifact size. The plan can be retried once with
// the same generated idempotency key; the PUT is never retried after an
// ambiguous response; the final commit can be retried with its opaque receipt.
func (c *Client) Publish(ctx context.Context, filePath string, opts Options) (Receipt, error) {
	if err := validateOptions(opts); err != nil {
		return Receipt{}, err
	}
	var expiresAt string
	if opts.ExpiresIn != 0 {
		expiresAt = c.now().UTC().Add(opts.ExpiresIn).Format(time.RFC3339Nano)
	}
	source, err := openBoundedRegularFile(filePath)
	if err != nil {
		return Receipt{}, err
	}
	defer source.close()

	idempotencyKey, err := newUUID()
	if err != nil {
		return Receipt{}, err
	}
	plan, err := c.plan(ctx, opts, source.sha, source.size, idempotencyKey, expiresAt)
	if err != nil {
		return Receipt{}, err
	}
	if err := c.upload(ctx, plan.Upload, source); err != nil {
		return Receipt{}, err
	}
	committed, err := c.commitWithRetry(ctx, opts, plan.Receipt)
	if err != nil {
		return Receipt{}, err
	}
	if err := validateCommitted(committed, source.sha, source.size); err != nil {
		return Receipt{}, err
	}
	return Receipt{
		Version:      "filecheap-publish/1",
		ArtifactRef:  committed.ArtifactRef,
		SHA256:       source.sha,
		SizeBytes:    source.size,
		Verification: committed.Artifact.Verification,
		PublishedAt:  c.now().UTC().Format(time.RFC3339),
	}, nil
}

type planRequest struct {
	ContentType    string               `json:"contentType"`
	ExpiresAt      string               `json:"expiresAt,omitempty"`
	IdempotencyKey string               `json:"idempotencyKey"`
	Kind           string               `json:"kind"`
	Producer       artifactref.Producer `json:"producer"`
	SHA256         string               `json:"sha256"`
	SizeBytes      int64                `json:"sizeBytes"`
}

type transferGrant struct {
	ExpiresAt string            `json:"expiresAt"`
	Headers   map[string]string `json:"headers"`
	Method    string            `json:"method"`
	URL       string            `json:"url"`
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

type serviceResponse struct {
	Artifact    artifactDetails           `json:"artifact"`
	ArtifactRef artifactref.ArtifactRefV1 `json:"artifactRef"`
	Receipt     string                    `json:"receipt,omitempty"`
	Upload      *transferGrant            `json:"upload,omitempty"`
}

func (c *Client) plan(ctx context.Context, opts Options, sha string, size int64, idempotencyKey, expiresAt string) (serviceResponse, error) {
	body, err := json.Marshal(planRequest{ContentType: opts.ContentType, ExpiresAt: expiresAt, IdempotencyKey: idempotencyKey, Kind: opts.Kind, Producer: opts.Producer, SHA256: sha, SizeBytes: size})
	if err != nil {
		return serviceResponse{}, fmt.Errorf("encode publish plan: %w", err)
	}
	var response serviceResponse
	var requestErr error
	for attempt := 0; attempt < 2; attempt++ {
		response = serviceResponse{}
		requestErr = c.doJSON(ctx, http.MethodPost, endpoint(opts.ServiceURL, "/api/v1/artifacts/plans"), opts.Token, body, http.StatusCreated, &response)
		if requestErr == nil || !isRetryable(requestErr) {
			break
		}
	}
	if requestErr != nil {
		return serviceResponse{}, fmt.Errorf("plan artifact publication: %w", requestErr)
	}
	if response.Receipt == "" || response.Upload == nil {
		return serviceResponse{}, errors.New("artifact service plan did not include an upload receipt and grant")
	}
	if err := validatePlan(response, sha, size); err != nil {
		return serviceResponse{}, err
	}
	return response, nil
}

func newUUID() (string, error) {
	var bytes [16]byte
	if _, err := rand.Read(bytes[:]); err != nil {
		return "", fmt.Errorf("create idempotency key: %w", err)
	}
	bytes[6] = (bytes[6] & 0x0f) | 0x40
	bytes[8] = (bytes[8] & 0x3f) | 0x80
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x", bytes[0:4], bytes[4:6], bytes[6:8], bytes[8:10], bytes[10:16]), nil
}

func (c *Client) upload(ctx context.Context, grant *transferGrant, source *publishSource) error {
	if grant.Method != http.MethodPut || grant.URL == "" {
		return errors.New("artifact service returned an invalid upload grant")
	}
	u, err := url.Parse(grant.URL)
	if err != nil || !isTransferURLAllowed(u) {
		return errors.New("artifact service returned an unsafe upload URL")
	}
	if _, err := time.Parse(time.RFC3339, grant.ExpiresAt); err != nil {
		return errors.New("artifact service returned an invalid upload expiry")
	}
	if _, err := source.file.Seek(0, io.SeekStart); err != nil {
		return fmt.Errorf("rewind publish file: %w", err)
	}
	// Hash what is actually sent so a file replaced between the local digest
	// and the transfer can never be committed under the planned SHA-256.
	sent := &hashingReader{reader: io.LimitReader(source.file, source.size), digest: sha256.New()}
	reqCtx, cancel := context.WithTimeout(ctx, transferTimeoutFor(source.size))
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, http.MethodPut, grant.URL, sent)
	if err != nil {
		return fmt.Errorf("create direct upload request: %w", err)
	}
	req.ContentLength = source.size
	for name, value := range grant.Headers {
		if !allowedTransferHeader(name) || value == "" {
			return errors.New("artifact service returned an unsafe upload header")
		}
		req.Header.Set(name, value)
	}
	response, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("direct upload failed; do not retry the PUT because the outcome may be unknown: %w", err)
	}
	defer func() { _ = response.Body.Close() }()
	_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, maxTransferBody+1))
	// A repeated plan with the same idempotency key can point at an immutable
	// object already written before a lost response. Commit verifies its exact
	// SHA-256, so a conflict is safe to advance without retrying the PUT. The
	// body may not have been drained in that case, so the sent digest is only
	// meaningful for an accepted transfer.
	if response.StatusCode == http.StatusConflict {
		return nil
	}
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("direct upload returned unexpected status %d", response.StatusCode)
	}
	if sent.read != source.size || hex.EncodeToString(sent.digest.Sum(nil)) != source.sha {
		return errors.New("publish file changed while it was being uploaded; the planned artifact was not committed")
	}
	return nil
}

type hashingReader struct {
	reader io.Reader
	digest hash.Hash
	read   int64
}

func (r *hashingReader) Read(p []byte) (int, error) {
	n, err := r.reader.Read(p)
	if n > 0 {
		r.digest.Write(p[:n])
		r.read += int64(n)
	}
	return n, err
}

func transferTimeoutFor(size int64) time.Duration {
	timeout := transferBaseTimeout + time.Duration(size/minTransferBytesPerSecond)*time.Second
	if timeout > maxTransferTimeout {
		return maxTransferTimeout
	}
	return timeout
}

func (c *Client) commitWithRetry(ctx context.Context, opts Options, receipt string) (serviceResponse, error) {
	body, err := json.Marshal(map[string]string{"receipt": receipt})
	if err != nil {
		return serviceResponse{}, fmt.Errorf("encode artifact commit: %w", err)
	}
	for attempt := 0; attempt < 2; attempt++ {
		var response serviceResponse
		err := c.doJSON(ctx, http.MethodPost, endpoint(opts.ServiceURL, "/api/v1/artifacts/commits"), opts.Token, body, http.StatusOK, &response)
		if err == nil {
			return response, nil
		}
		if !isRetryable(err) || attempt == 1 {
			return serviceResponse{}, fmt.Errorf("commit artifact publication: %w", err)
		}
	}
	return serviceResponse{}, errors.New("commit artifact publication failed")
}

func (c *Client) doJSON(ctx context.Context, method, target, token string, body []byte, expectedStatus int, destination any) error {
	reqCtx, cancel := context.WithTimeout(ctx, controlTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, method, target, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create service request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	response, err := c.httpClient.Do(req)
	if err != nil {
		return transientError{err: err}
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != expectedStatus {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, maxControlBody+1))
		if response.StatusCode >= 500 {
			return transientError{err: fmt.Errorf("artifact service returned unexpected status %d", response.StatusCode)}
		}
		return fmt.Errorf("artifact service returned unexpected status %d", response.StatusCode)
	}
	limited := io.LimitReader(response.Body, maxControlBody+1)
	decoder := json.NewDecoder(limited)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return fmt.Errorf("decode artifact service response: %w", err)
	}
	if err := ensureEOF(decoder); err != nil {
		return err
	}
	return nil
}

func ensureEOF(decoder *json.Decoder) error {
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return errors.New("artifact service response contains trailing JSON")
		}
		return fmt.Errorf("decode artifact service response: %w", err)
	}
	return nil
}

func validateOptions(opts Options) error {
	if !publisherTokenPattern.MatchString(opts.Token) {
		return errors.New("FILECHEAP_INGEST_TOKEN must be a 43-128 character base64url publisher credential")
	}
	if os.Getenv("VERCEL") != "" || os.Getenv("VERCEL_OIDC_TOKEN") != "" {
		return errors.New("fcheap publish is a local or droplet command and does not accept Vercel credentials")
	}
	if _, err := parseServiceURL(opts.ServiceURL); err != nil {
		return err
	}
	if opts.ContentType == "" || len(opts.ContentType) > 255 {
		return errors.New("publish content type must be between 1 and 255 characters")
	}
	if opts.ExpiresIn != 0 && (opts.ExpiresIn < minRetention || opts.ExpiresIn > maxRetention) {
		return fmt.Errorf("publish retention must be between %s and %s", minRetention, maxRetention)
	}
	if opts.Kind == "" {
		return errors.New("publish kind is required")
	}
	if opts.Producer.Tool == "" {
		return errors.New("publish producer tool is required")
	}
	if opts.Producer.NativeSchema == "" {
		return errors.New("publish native schema is required by the producer policy")
	}
	if _, err := artifactref.NewCloud(
		"private",
		"art_abcdefghijklmnop",
		opts.Kind,
		opts.Producer,
	); err != nil {
		return fmt.Errorf("validate publish routing metadata: %w", err)
	}
	return nil
}

// publishSource is one bounded regular file, already hashed in a single
// streaming pass and rewound so the direct PUT can stream the same bytes.
type publishSource struct {
	file *os.File
	sha  string
	size int64
}

func (s *publishSource) close() {
	if s != nil && s.file != nil {
		_ = s.file.Close()
	}
}

func openBoundedRegularFile(filePath string) (*publishSource, error) {
	return openBoundedRegularFileWithHooks(filePath, boundedFileReadHooks{})
}

type boundedFileReadHooks struct {
	afterInspect func() error
	beforeRead   func() error
}

func openBoundedRegularFileWithHooks(filePath string, hooks boundedFileReadHooks) (source *publishSource, err error) {
	info, err := os.Lstat(filePath)
	if err != nil {
		return nil, fmt.Errorf("inspect publish file: %w", err)
	}
	if !info.Mode().IsRegular() {
		return nil, errors.New("publish accepts one regular file, not a directory, device, or symlink")
	}
	if info.Size() <= 0 || info.Size() > MaxBytes {
		return nil, fmt.Errorf("publish file must be between 1 byte and %d bytes", MaxBytes)
	}
	if hooks.afterInspect != nil {
		if err := hooks.afterInspect(); err != nil {
			return nil, fmt.Errorf("prepare publish file read: %w", err)
		}
	}
	file, err := openPublishFileNoFollow(filePath)
	if err != nil {
		return nil, fmt.Errorf("open publish file without following links: %w", err)
	}
	defer func() {
		if source == nil {
			_ = file.Close()
		}
	}()

	openedInfo, err := file.Stat()
	if err != nil {
		return nil, fmt.Errorf("inspect opened publish file: %w", err)
	}
	// Bind the descriptor to the regular inode inspected above before reading
	// any bytes. This makes path replacement unable to redirect a publication.
	if !openedInfo.Mode().IsRegular() || !os.SameFile(info, openedInfo) {
		return nil, errors.New("publish file changed before it could be read")
	}
	if openedInfo.Size() <= 0 || openedInfo.Size() > MaxBytes {
		return nil, fmt.Errorf("publish file must be between 1 byte and %d bytes", MaxBytes)
	}
	if hooks.beforeRead != nil {
		if err := hooks.beforeRead(); err != nil {
			return nil, fmt.Errorf("prepare bounded publish file read: %w", err)
		}
	}
	// Digest the file incrementally through a fixed buffer: memory stays
	// constant no matter how close the artifact is to MaxBytes.
	digest := sha256.New()
	read, err := io.CopyBuffer(digest, io.LimitReader(file, MaxBytes+1), make([]byte, hashBufferBytes))
	if err != nil {
		return nil, fmt.Errorf("read publish file: %w", err)
	}
	afterReadInfo, err := file.Stat()
	if err != nil {
		return nil, fmt.Errorf("inspect publish file after read: %w", err)
	}
	if read != openedInfo.Size() ||
		read > MaxBytes ||
		afterReadInfo.Size() != openedInfo.Size() ||
		!afterReadInfo.ModTime().Equal(openedInfo.ModTime()) {
		return nil, errors.New("publish file changed while it was being read")
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return nil, fmt.Errorf("rewind publish file: %w", err)
	}
	return &publishSource{file: file, sha: hex.EncodeToString(digest.Sum(nil)), size: read}, nil
}

func validatePlan(response serviceResponse, sha string, size int64) error {
	if response.Artifact.State != "planned" || response.Artifact.SHA256 != sha || response.Artifact.SizeBytes != size || response.Upload == nil {
		return errors.New("artifact service plan does not match the local file")
	}
	if err := response.ArtifactRef.Validate(); err != nil {
		return fmt.Errorf("validate artifact reference: %w", err)
	}
	if response.ArtifactRef.Provider != artifactref.ProviderCloud || response.ArtifactRef.ArtifactID != response.Artifact.ArtifactID || response.ArtifactRef.Kind != response.Artifact.Kind {
		return errors.New("artifact service plan returned a non-canonical artifact reference")
	}
	return nil
}

func validateCommitted(response serviceResponse, sha string, size int64) error {
	if response.Artifact.State != "committed" || response.Artifact.SHA256 != sha || response.Artifact.SizeBytes != size || response.Artifact.Verification != "server-sha256" {
		return errors.New("artifact service commit does not verify the local file")
	}
	if err := response.ArtifactRef.Validate(); err != nil {
		return fmt.Errorf("validate artifact reference: %w", err)
	}
	if response.ArtifactRef.Provider != artifactref.ProviderCloud || response.ArtifactRef.ArtifactID != response.Artifact.ArtifactID {
		return errors.New("artifact service commit returned a non-canonical artifact reference")
	}
	if strings.Contains(response.ArtifactRef.URI, "?") || strings.Contains(response.ArtifactRef.WebURL, "?") {
		return errors.New("artifact service commit returned a signed URL in the artifact reference")
	}
	return nil
}

func parseServiceURL(raw string) (*url.URL, error) {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme == "" || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Path != "" && u.Path != "/") {
		return nil, errors.New("FILECHEAP_ARTIFACT_SERVICE_URL must be a bare HTTPS origin, or HTTP loopback for local tests")
	}
	if u.Scheme != "https" && (u.Scheme != "http" || !isLoopbackHost(u.Hostname())) {
		return nil, errors.New("FILECHEAP_ARTIFACT_SERVICE_URL must use HTTPS outside loopback")
	}
	return u, nil
}

func endpoint(base, suffix string) string {
	u, _ := parseServiceURL(base)
	u.Path = path.Clean(suffix)
	return u.String()
}

func isTransferURLAllowed(u *url.URL) bool {
	return u != nil && u.User == nil && u.Fragment == "" && (u.Scheme == "https" || (u.Scheme == "http" && isLoopbackHost(u.Hostname())))
}
func isLoopbackHost(host string) bool {
	return host == "localhost" || host == "127.0.0.1" || host == "::1"
}
func allowedTransferHeader(name string) bool { return strings.EqualFold(name, "content-type") }

type transientError struct{ err error }

func (e transientError) Error() string { return e.err.Error() }
func (e transientError) Unwrap() error { return e.err }
func isRetryable(err error) bool {
	var temporary transientError
	return errors.As(err, &temporary)
}

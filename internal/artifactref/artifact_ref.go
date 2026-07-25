// Package artifactref defines portable references to artifacts managed by
// file.cheap or an external link. Constructors currently emit local refs only.
package artifactref

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"path"
	"regexp"
	"strconv"
	"strings"
)

const (
	// SchemaURI identifies the ArtifactRefV1 wire contract.
	SchemaURI = "urn:filecheap.dev:artifact-ref:v1"
	// Version is the only ArtifactRef version supported by this package.
	Version = 1
	// ProviderLocal identifies a stash in the local file.cheap vault.
	ProviderLocal = "fcheap-local"
	// ProviderCloud identifies an artifact in the future hosted file.cheap vault.
	ProviderCloud = "fcheap-cloud"
	// ProviderLink identifies a stable external HTTP(S) artifact.
	ProviderLink = "link"

	MaxArtifactIDLength       = 99
	MaxRemoteArtifactIDLength = 160
	MaxKindLength             = 128
	MaxToolLength             = 64
	MaxVersionLength          = 64
	MaxNativeSchema           = 256
	MaxNativeIDLength         = 160
	MaxEntrypointLength       = 512
	MaxURILength              = 2048
)

var (
	artifactIDPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)
	kindPattern       = regexp.MustCompile(`^[a-z][a-z0-9]*(?:[._-][a-z0-9]+)*$`)
	toolPattern       = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)
	versionPattern    = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._+-]*$`)
	nativeIDPattern   = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:-]*$`)
	pathSegment       = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)
	remoteIDPattern   = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)
	stableHTTPPattern = regexp.MustCompile(`^https?://[A-Za-z0-9.-]+(?::[0-9]{1,5})?(?:/[A-Za-z0-9._~!$&'()*+,;=:@/-]*)?$`)
)

// ArtifactRefV1 is a stable reference with credential-free transport fields.
// Caller-supplied metadata is syntax-validated but is not DLP-scanned.
//
// Integrity is intentionally absent: the legacy manifest ContentHash is not a
// portable tree or bundle digest. WebURL is allowed only as a convenience for
// cloud refs; local and link refs omit it.
type ArtifactRefV1 struct {
	Schema     string    `json:"$schema"`
	Version    int       `json:"version"`
	Provider   string    `json:"provider"`
	URI        string    `json:"uri"`
	ArtifactID string    `json:"artifact_id,omitempty"`
	Kind       string    `json:"kind"`
	Producer   *Producer `json:"producer,omitempty"`
	WebURL     string    `json:"web_url,omitempty"`
}

// Producer contains bounded routing metadata for the native artifact stored
// inside a stash. Tool is required whenever producer metadata is present.
type Producer struct {
	Tool         string `json:"tool"`
	Version      string `json:"version,omitempty"`
	NativeSchema string `json:"native_schema,omitempty"`
	NativeID     string `json:"native_id,omitempty"`
	Entrypoint   string `json:"entrypoint,omitempty"`
}

// LocalOptions controls optional metadata on a local ArtifactRefV1. A zero
// Producer value omits producer entirely.
type LocalOptions struct {
	Kind     string
	Producer Producer
}

// NewCloud constructs and validates a credential-free reference returned by the
// private artifact service. Transfer URLs are deliberately not part of this
// contract and must never be copied into an ArtifactRefV1.
func NewCloud(vaultID, artifactID, kind string, producer Producer) (ArtifactRefV1, error) {
	ref := ArtifactRefV1{
		Schema:     SchemaURI,
		Version:    Version,
		Provider:   ProviderCloud,
		URI:        "fcheap://cloud/vaults/" + vaultID + "/artifacts/" + artifactID,
		ArtifactID: artifactID,
		Kind:       kind,
		Producer:   &producer,
	}
	if producer == (Producer{}) {
		ref.Producer = nil
	}
	if err := ref.Validate(); err != nil {
		return ArtifactRefV1{}, err
	}
	return ref, nil
}

// NewLocal constructs and validates a reference to an existing local stash.
func NewLocal(artifactID, bundleType string, opts LocalOptions) (ArtifactRefV1, error) {
	kind := opts.Kind
	if kind == "" {
		kind = DefaultKind(bundleType)
	}

	ref := ArtifactRefV1{
		Schema:     SchemaURI,
		Version:    Version,
		Provider:   ProviderLocal,
		URI:        LocalURI(artifactID),
		ArtifactID: artifactID,
		Kind:       kind,
	}
	if opts.Producer != (Producer{}) {
		producer := opts.Producer
		ref.Producer = &producer
	}
	if err := ref.Validate(); err != nil {
		return ArtifactRefV1{}, err
	}
	return ref, nil
}

// LocalURI returns the stable local URI for a stash ID. Callers should use
// NewLocal when they need validation.
func LocalURI(artifactID string) string {
	return "fcheap://stash/" + artifactID
}

// DefaultKind derives a conservative kind from a detected bundle type.
// Generic, empty, or malformed legacy bundle values remain ordinary stashes.
func DefaultKind(bundleType string) string {
	bundleType = strings.ToLower(strings.TrimSpace(bundleType))
	if bundleType == "" || bundleType == "generic" {
		return "filecheap.stash"
	}
	candidate := bundleType + ".bundle"
	if validKind(candidate) {
		return candidate
	}
	return "filecheap.stash"
}

// ParseJSON decodes one strict ArtifactRefV1 JSON object. Unknown fields and
// trailing JSON values are rejected to match the published JSON Schema.
func ParseJSON(data []byte) (ArtifactRefV1, error) {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()

	var ref ArtifactRefV1
	if err := decoder.Decode(&ref); err != nil {
		return ArtifactRefV1{}, fmt.Errorf("decode artifact ref: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return ArtifactRefV1{}, fmt.Errorf("decode artifact ref: trailing JSON value")
		}
		return ArtifactRefV1{}, fmt.Errorf("decode artifact ref: %w", err)
	}
	if err := validateJSONPresence(data, ref); err != nil {
		return ArtifactRefV1{}, err
	}
	if err := ref.Validate(); err != nil {
		return ArtifactRefV1{}, err
	}
	return ref, nil
}

// Validate enforces the provider-specific ArtifactRefV1 contract.
func (r ArtifactRefV1) Validate() error {
	if r.Schema != SchemaURI {
		return invalid(".$schema", fmt.Sprintf("must be %q", SchemaURI))
	}
	if r.Version != Version {
		return invalid(".version", fmt.Sprintf("must be %d", Version))
	}
	if !validKind(r.Kind) {
		return invalid(".kind", "must be a bounded lowercase namespaced token")
	}
	if r.Producer != nil {
		if err := r.Producer.validate(); err != nil {
			return err
		}
	}
	switch r.Provider {
	case ProviderLocal:
		return r.validateLocal()
	case ProviderCloud:
		return r.validateCloud()
	case ProviderLink:
		return r.validateLink()
	default:
		return invalid(".provider", "must be fcheap-local, fcheap-cloud, or link")
	}
}

func (r ArtifactRefV1) validateLocal() error {
	if err := validateLocalArtifactID(r.ArtifactID); err != nil {
		return err
	}
	if r.URI != LocalURI(r.ArtifactID) {
		return invalid(".uri", "must exactly match fcheap://stash/<artifact_id>")
	}
	if r.WebURL != "" {
		return invalid(".web_url", "must be omitted for fcheap-local")
	}
	return nil
}

func (r ArtifactRefV1) validateCloud() error {
	if err := validateRemoteArtifactID(r.ArtifactID); err != nil {
		return err
	}
	parsed, err := url.Parse(r.URI)
	if err != nil || parsed.Scheme != "fcheap" || parsed.Host != "cloud" ||
		parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" || parsed.RawPath != "" {
		return invalid(".uri", "must be a canonical fcheap cloud artifact URI")
	}
	parts := strings.Split(strings.TrimPrefix(parsed.Path, "/"), "/")
	if len(parts) != 4 || parts[0] != "vaults" || parts[2] != "artifacts" ||
		!validRemoteID(parts[1]) || parts[3] != r.ArtifactID {
		return invalid(".uri", "must exactly match fcheap://cloud/vaults/<vault-id>/artifacts/<artifact_id>")
	}
	if r.WebURL != "" {
		if err := validateStableHTTPURL(".web_url", r.WebURL, true); err != nil {
			return err
		}
	}
	return nil
}

func (r ArtifactRefV1) validateLink() error {
	if r.ArtifactID != "" {
		return invalid(".artifact_id", "must be omitted for link")
	}
	if r.WebURL != "" {
		return invalid(".web_url", "must be omitted for link")
	}
	return validateStableHTTPURL(".uri", r.URI, false)
}

func validateLocalArtifactID(id string) error {
	if id == "" || len(id) > MaxArtifactIDLength || !artifactIDPattern.MatchString(id) ||
		strings.Contains(id, ":") || id == "." || id == ".." {
		return invalid(".artifact_id", "must be 1-99 portable ASCII characters without ':'")
	}
	return nil
}

func validateRemoteArtifactID(id string) error {
	if !validRemoteID(id) {
		return invalid(".artifact_id", "must be a portable token of at most 160 characters")
	}
	return nil
}

func validRemoteID(id string) bool {
	return id != "" && len(id) <= MaxRemoteArtifactIDLength && remoteIDPattern.MatchString(id) &&
		id != "." && id != ".."
}

func validKind(kind string) bool {
	return kind != "" && len(kind) <= MaxKindLength && kindPattern.MatchString(kind)
}

func (p Producer) validate() error {
	if p.Tool == "" || len(p.Tool) > MaxToolLength || !toolPattern.MatchString(p.Tool) {
		return invalid(".producer.tool", "is required and must be a portable tool token of at most 64 characters")
	}
	if p.Version != "" && (len(p.Version) > MaxVersionLength || !versionPattern.MatchString(p.Version)) {
		return invalid(".producer.version", "must be a portable version token of at most 64 characters")
	}
	if p.NativeSchema != "" {
		if len(p.NativeSchema) > MaxNativeSchema || !asciiVisible(p.NativeSchema) {
			return invalid(".producer.native_schema", "must be a URN or HTTPS URI of at most 256 ASCII characters")
		}
		parsed, err := url.Parse(p.NativeSchema)
		if err != nil || !parsed.IsAbs() || parsed.User != nil || parsed.RawQuery != "" ||
			(parsed.Scheme == "urn" && parsed.Opaque == "") ||
			(parsed.Scheme == "https" && parsed.Host == "") ||
			(parsed.Scheme != "urn" && parsed.Scheme != "https") {
			return invalid(".producer.native_schema", "must be a URN or HTTPS URI without credentials or a query string")
		}
	}
	if p.NativeID != "" && (len(p.NativeID) > MaxNativeIDLength || !nativeIDPattern.MatchString(p.NativeID)) {
		return invalid(".producer.native_id", "must be a portable token of at most 160 characters")
	}
	if p.Entrypoint != "" {
		if err := validateEntrypoint(p.Entrypoint); err != nil {
			return err
		}
	}
	return nil
}

func validateEntrypoint(entrypoint string) error {
	if len(entrypoint) > MaxEntrypointLength || strings.Contains(entrypoint, `\`) ||
		path.IsAbs(entrypoint) || path.Clean(entrypoint) != entrypoint {
		return invalid(".producer.entrypoint", "must be a safe slash-separated relative path")
	}
	for _, segment := range strings.Split(entrypoint, "/") {
		if segment == "" || segment == "." || segment == ".." || !pathSegment.MatchString(segment) {
			return invalid(".producer.entrypoint", "must be a safe slash-separated relative path")
		}
	}
	return nil
}

func validateStableHTTPURL(field, value string, httpsOnly bool) error {
	if value == "" || len(value) > MaxURILength || !asciiVisible(value) || !stableHTTPPattern.MatchString(value) {
		return invalid(field, "must be a bounded absolute HTTP(S) URL")
	}
	parsed, err := url.Parse(value)
	if err != nil || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" ||
		parsed.Fragment != "" || parsed.RawPath != "" {
		return invalid(field, "must be a stable HTTP(S) URL without credentials, query string, or fragment")
	}
	if (httpsOnly && parsed.Scheme != "https") ||
		(!httpsOnly && parsed.Scheme != "http" && parsed.Scheme != "https") {
		return invalid(field, "must use an allowed HTTP(S) scheme")
	}
	if port := parsed.Port(); port != "" {
		number, err := strconv.Atoi(port)
		if err != nil || number > 65535 {
			return invalid(field, "must use a port between 0 and 65535")
		}
	}
	return nil
}

func validateJSONPresence(data []byte, ref ArtifactRefV1) error {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return fmt.Errorf("decode artifact ref fields: %w", err)
	}
	if _, present := raw["artifact_id"]; present && ref.ArtifactID == "" {
		return invalid(".artifact_id", "must be omitted rather than empty")
	}
	if _, present := raw["web_url"]; present && ref.WebURL == "" {
		return invalid(".web_url", "must be omitted rather than empty")
	}
	rawProducer, present := raw["producer"]
	if !present {
		return nil
	}
	if ref.Producer == nil {
		return invalid(".producer", "must be an object when present")
	}
	var producer map[string]json.RawMessage
	if err := json.Unmarshal(rawProducer, &producer); err != nil {
		return invalid(".producer", "must be an object when present")
	}
	for field, value := range map[string]string{
		"tool":          ref.Producer.Tool,
		"version":       ref.Producer.Version,
		"native_schema": ref.Producer.NativeSchema,
		"native_id":     ref.Producer.NativeID,
		"entrypoint":    ref.Producer.Entrypoint,
	} {
		if _, present := producer[field]; present && value == "" {
			return invalid(".producer."+field, "must be omitted rather than empty")
		}
	}
	return nil
}

func asciiVisible(value string) bool {
	for i := 0; i < len(value); i++ {
		if value[i] < 0x21 || value[i] > 0x7e {
			return false
		}
	}
	return true
}

// ValidationError identifies one field that violates the wire contract.
type ValidationError struct {
	Field   string
	Problem string
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("invalid artifact ref %s: %s", e.Field, e.Problem)
}

func invalid(field, problem string) error {
	return &ValidationError{Field: field, Problem: problem}
}

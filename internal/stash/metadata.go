package stash

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
	"unicode"

	"github.com/abdul-hamid-achik/file.cheap/internal/apperror"
)

// Limits for caller-supplied manifest metadata. Metadata is an index aid
// (e.g. a dataset version or the commit a run measured), not a payload store.
const (
	MaxMetadataEntries    = 32
	MaxMetadataValueBytes = 256
)

var metadataKeyPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9_.-]{0,63}$`)

// reservedMetadataKeys are manifest custom fields that file.cheap writes
// itself. Callers must not set them: a forged secrets_found or indexed flag
// would misreport the save-time scan or index state.
var reservedMetadataKeys = map[string]bool{
	"source":        true, // set through --source / the source input
	"indexed":       true,
	"indexed_files": true,
	"secrets_found": true,
	"secrets_rules": true,
	// The scan accounting is written by Save; a forged value would claim
	// coverage the scan never had.
	"secrets_files_scanned": true,
	"secrets_files_skipped": true,
	"source_video":          true,
	"duration_seconds":      true,
	"frame_rate":            true,
}

// IsReservedMetadataKey reports whether key is written by file.cheap itself.
func IsReservedMetadataKey(key string) bool {
	return reservedMetadataKeys[key]
}

// ValidateMetadata checks caller-supplied key/value metadata before it is
// merged into a manifest's custom fields.
func ValidateMetadata(meta map[string]string) error {
	if len(meta) > MaxMetadataEntries {
		return apperror.New("invalid_input", fmt.Sprintf("at most %d metadata entries are allowed, got %d", MaxMetadataEntries, len(meta)))
	}
	keys := make([]string, 0, len(meta))
	for key := range meta {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		if !metadataKeyPattern.MatchString(key) {
			return apperror.New("invalid_input", fmt.Sprintf("metadata key %q must match %s", key, metadataKeyPattern.String()))
		}
		if reservedMetadataKeys[key] {
			return apperror.New("invalid_input", fmt.Sprintf("metadata key %q is reserved by file.cheap", key))
		}
		value := meta[key]
		if len(value) > MaxMetadataValueBytes {
			return apperror.New("invalid_input", fmt.Sprintf("metadata value for %q exceeds %d bytes", key, MaxMetadataValueBytes))
		}
		if strings.IndexFunc(value, unicode.IsControl) >= 0 {
			return apperror.New("invalid_input", fmt.Sprintf("metadata value for %q contains control characters", key))
		}
	}
	return nil
}

// ParseMetadata turns repeated key=value arguments into a validated map.
// A key may appear only once so a typo cannot silently overwrite a value.
func ParseMetadata(pairs []string) (map[string]string, error) {
	if len(pairs) == 0 {
		return nil, nil
	}
	meta := make(map[string]string, len(pairs))
	for _, pair := range pairs {
		key, value, ok := strings.Cut(pair, "=")
		if !ok || key == "" {
			return nil, apperror.New("invalid_input", fmt.Sprintf("metadata %q must be key=value", pair))
		}
		if _, dup := meta[key]; dup {
			return nil, apperror.New("invalid_input", fmt.Sprintf("metadata key %q given more than once", key))
		}
		meta[key] = value
	}
	if err := ValidateMetadata(meta); err != nil {
		return nil, err
	}
	return meta, nil
}

// MergeMetadata returns base (may be nil) with validated meta added. It never
// overwrites an existing key.
func MergeMetadata(base, meta map[string]string) (map[string]string, error) {
	if err := ValidateMetadata(meta); err != nil {
		return nil, err
	}
	if len(meta) == 0 {
		return base, nil
	}
	out := make(map[string]string, len(base)+len(meta))
	for k, v := range base {
		out[k] = v
	}
	for k, v := range meta {
		if _, exists := out[k]; exists {
			return nil, apperror.New("invalid_input", fmt.Sprintf("metadata key %q conflicts with an existing field", k))
		}
		out[k] = v
	}
	return out, nil
}

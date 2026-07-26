package publish

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/abdul-hamid-achik/file.cheap/internal/artifactref"
)

const testPublisherToken = "iiiiiiiiiiiiiiiiiiiiiiiiiiiiiiiiiiiiiiiiiii"

func TestPublishUsesExactPlanUploadCommitContract(t *testing.T) {
	t.Parallel()
	contents := []byte("bounded private artifact")
	digest := sha256.Sum256(contents)
	sha := hex.EncodeToString(digest[:])
	producer := artifactref.Producer{
		Tool:         "chalupa",
		NativeSchema: "urn:chalupa:log-chunk:v1",
	}
	ref, err := artifactref.NewCloud("private", "art_abcdefghijklmnop", "chalupa.log-chunk", producer)
	if err != nil {
		t.Fatal(err)
	}
	var server *httptest.Server
	expiresAt := "2026-07-31T12:00:00Z"
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/artifacts/plans":
			if r.Method != http.MethodPost || r.Header.Get("Authorization") != "Bearer "+testPublisherToken {
				t.Fatalf("unexpected plan request")
			}
			var got planRequest
			if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
				t.Fatal(err)
			}
			if got.SHA256 != sha || got.SizeBytes != int64(len(contents)) || got.ExpiresAt != expiresAt {
				t.Fatalf("plan did not bind local bytes")
			}
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(map[string]any{"artifact": map[string]any{"artifactId": ref.ArtifactID, "committedAt": nil, "contentType": "application/zstd", "expiresAt": expiresAt, "kind": ref.Kind, "producer": ref.Producer, "sha256": sha, "sizeBytes": len(contents), "state": "planned", "verification": "server-sha256"}, "artifactRef": ref, "receipt": "123e4567-e89b-12d3-a456-426614174000", "upload": map[string]any{"expiresAt": "2030-01-01T00:00:00Z", "headers": map[string]string{"content-type": "application/zstd"}, "method": "PUT", "url": server.URL + "/direct?grant=opaque"}})
		case "/direct":
			if r.Method != http.MethodPut || r.Header.Get("Content-Type") != "application/zstd" {
				t.Fatalf("unexpected direct upload")
			}
			got, err := io.ReadAll(r.Body)
			if err != nil {
				t.Fatal(err)
			}
			if string(got) != string(contents) {
				t.Fatalf("uploaded bytes changed")
			}
			w.WriteHeader(http.StatusOK)
		case "/api/v1/artifacts/commits":
			if r.Method != http.MethodPost || r.Header.Get("Authorization") != "Bearer "+testPublisherToken {
				t.Fatalf("unexpected commit request")
			}
			committedAt := "2026-07-24T12:00:01Z"
			_ = json.NewEncoder(w).Encode(map[string]any{"artifact": map[string]any{"artifactId": ref.ArtifactID, "committedAt": committedAt, "contentType": "application/zstd", "expiresAt": expiresAt, "kind": ref.Kind, "producer": ref.Producer, "sha256": sha, "sizeBytes": len(contents), "state": "committed", "verification": "server-sha256"}, "artifactRef": ref})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	path := filepath.Join(t.TempDir(), "artifact.zst")
	if err := os.WriteFile(path, contents, 0o600); err != nil {
		t.Fatal(err)
	}
	client := NewClient(server.Client())
	client.now = func() time.Time {
		return time.Date(2026, 7, 24, 12, 0, 0, 0, time.UTC)
	}
	receipt, err := client.Publish(context.Background(), path, Options{ContentType: "application/zstd", ExpiresIn: 7 * 24 * time.Hour, Kind: "chalupa.log-chunk", Producer: producer, ServiceURL: server.URL, Token: testPublisherToken})
	if err != nil {
		t.Fatal(err)
	}
	if receipt.ArtifactRef.URI != ref.URI || receipt.SHA256 != sha || receipt.Verification != "server-sha256" {
		t.Fatalf("unexpected receipt: %#v", receipt)
	}
}

func TestPublishRejectsInvalidRetentionBeforeReadingInput(t *testing.T) {
	t.Parallel()
	client := NewClient(http.DefaultClient)
	_, err := client.Publish(context.Background(), filepath.Join(t.TempDir(), "missing"), Options{
		ContentType: "application/octet-stream",
		ExpiresIn:   32 * 24 * time.Hour,
		Kind:        "filecheap.artifact",
		Producer:    artifactref.Producer{Tool: "fcheap", NativeSchema: "urn:filecheap.dev:artifact:v1"},
		ServiceURL:  "https://file.cheap",
		Token:       testPublisherToken,
	})
	if err == nil || !strings.Contains(err.Error(), "retention") {
		t.Fatalf("expected retention validation error, got %v", err)
	}
}

func TestPublishRejectsSignedReferenceAndPreservesInput(t *testing.T) {
	t.Parallel()
	contents := []byte("input must remain")
	path := filepath.Join(t.TempDir(), "artifact")
	if err := os.WriteFile(path, contents, 0o600); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Error(w, "no", http.StatusUnauthorized) }))
	defer server.Close()
	_, err := NewClient(server.Client()).Publish(context.Background(), path, Options{ContentType: "text/plain", Kind: "cairntrace.run", Producer: artifactref.Producer{Tool: "cairntrace", NativeSchema: "urn:cairntrace.dev:run:v1"}, ServiceURL: server.URL, Token: testPublisherToken})
	if err == nil {
		t.Fatal("expected publish failure")
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != string(contents) {
		t.Fatal("publish changed the local input")
	}
}

func TestPublishRejectsWeakPublisherCredentialBeforeReadingInput(t *testing.T) {
	t.Parallel()
	_, err := NewClient(http.DefaultClient).Publish(
		context.Background(),
		filepath.Join(t.TempDir(), "missing"),
		Options{
			ContentType: "application/octet-stream",
			Kind:        "filecheap.artifact",
			Producer:    artifactref.Producer{Tool: "fcheap", NativeSchema: "urn:filecheap.dev:artifact:v1"},
			ServiceURL:  "https://file.cheap",
			Token:       "weak",
		},
	)
	if err == nil || !strings.Contains(err.Error(), "43-128 character base64url") {
		t.Fatalf("expected publisher credential validation error, got %v", err)
	}
}

func TestReadBoundedRegularFileRejectsPathSwapToSymlink(t *testing.T) {
	t.Parallel()
	directory := t.TempDir()
	filePath := filepath.Join(directory, "artifact")
	originalPath := filepath.Join(directory, "artifact.original")
	secretPath := filepath.Join(directory, "secret")
	if err := os.WriteFile(filePath, []byte("allowed"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(secretPath, []byte("private"), 0o600); err != nil {
		t.Fatal(err)
	}

	source, err := openBoundedRegularFileWithHooks(
		filePath,
		boundedFileReadHooks{
			afterInspect: func() error {
				if err := os.Rename(filePath, originalPath); err != nil {
					return err
				}
				if err := os.Symlink(secretPath, filePath); err != nil {
					t.Skipf("symlinks are unavailable on this platform: %v", err)
				}
				return nil
			},
		},
	)
	defer source.close()
	if err == nil {
		t.Fatal("expected a path replacement to be rejected")
	}
	if source != nil {
		t.Fatalf("path replacement returned an open publish source")
	}
}

func TestReadBoundedRegularFileCapsGrowthAfterOpen(t *testing.T) {
	t.Parallel()
	filePath := filepath.Join(t.TempDir(), "artifact")
	if err := os.WriteFile(filePath, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}

	source, err := openBoundedRegularFileWithHooks(
		filePath,
		boundedFileReadHooks{
			beforeRead: func() error {
				return os.Truncate(filePath, MaxBytes+1024)
			},
		},
	)
	defer source.close()
	if err == nil || !strings.Contains(err.Error(), "changed while it was being read") {
		t.Fatalf("expected bounded growth rejection, got %v", err)
	}
	if source != nil {
		t.Fatalf("oversized growth returned an open publish source")
	}
}

func TestOpenBoundedRegularFileStreamsLargeArtifactDigest(t *testing.T) {
	t.Parallel()
	// Comfortably past the retired 2 MiB read-everything bound.
	contents := make([]byte, 5*1024*1024+7)
	for index := range contents {
		contents[index] = byte(index % 251)
	}
	filePath := filepath.Join(t.TempDir(), "artifact.bin")
	if err := os.WriteFile(filePath, contents, 0o600); err != nil {
		t.Fatal(err)
	}

	source, err := openBoundedRegularFile(filePath)
	if err != nil {
		t.Fatal(err)
	}
	defer source.close()

	digest := sha256.Sum256(contents)
	if source.sha != hex.EncodeToString(digest[:]) {
		t.Fatalf("streamed digest does not match the file")
	}
	if source.size != int64(len(contents)) {
		t.Fatalf("unexpected streamed size %d", source.size)
	}
	// The source must be rewound so the direct PUT streams the same bytes.
	streamed, err := io.ReadAll(source.file)
	if err != nil {
		t.Fatal(err)
	}
	if len(streamed) != len(contents) {
		t.Fatalf("publish source was not rewound: read %d bytes", len(streamed))
	}
}

func TestPublishStreamsLargeArtifactWithoutBuffering(t *testing.T) {
	t.Parallel()
	contents := make([]byte, 3*1024*1024+11)
	for index := range contents {
		contents[index] = byte((index * 7) % 253)
	}
	digest := sha256.Sum256(contents)
	sha := hex.EncodeToString(digest[:])
	producer := artifactref.Producer{Tool: "cairntrace", NativeSchema: "urn:cairntrace.dev:run:v1", NativeID: "run-123"}
	runIndex := json.RawMessage(`{"$schema":"urn:filecheap.dev:run-index:v1","version":1,"detector":{"name":"cairntrace-run","version":"1"},"run":{"nativeId":"run-123","seriesKey":"series_key_123456","status":"passed"},"health":{"state":"ok","reasons":[],"declared":0,"present":0,"empty":0,"missing":0,"changed":0},"counts":{"steps":0,"outcomes":0,"artifacts":0},"outcomes":[],"evidence":[]}`)
	ref, err := artifactref.NewCloud("private", "art_abcdefghijklmnop", "cairntrace.run", producer)
	if err != nil {
		t.Fatal(err)
	}
	var server *httptest.Server
	var uploadedSHA string
	var uploadedBytes int64
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/artifacts/plans":
			var got planRequest
			if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
				t.Error(err)
				return
			}
			if got.SizeBytes != int64(len(contents)) || got.SHA256 != sha || !bytes.Equal(got.RunIndex, runIndex) {
				t.Errorf("plan did not bind the streamed bytes")
			}
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(map[string]any{"artifact": map[string]any{"artifactId": ref.ArtifactID, "committedAt": nil, "contentType": "application/gzip", "expiresAt": nil, "kind": ref.Kind, "producer": ref.Producer, "sha256": sha, "sizeBytes": len(contents), "state": "planned", "verification": "server-sha256"}, "artifactRef": ref, "receipt": "123e4567-e89b-12d3-a456-426614174000", "upload": map[string]any{"expiresAt": "2030-01-01T00:00:00Z", "headers": map[string]string{"content-type": "application/gzip"}, "method": "PUT", "url": server.URL + "/direct"}})
		case "/direct":
			if r.ContentLength != int64(len(contents)) {
				t.Errorf("direct upload did not declare an exact content length: %d", r.ContentLength)
			}
			streamed := sha256.New()
			written, err := io.Copy(streamed, r.Body)
			if err != nil {
				t.Error(err)
				return
			}
			uploadedBytes = written
			uploadedSHA = hex.EncodeToString(streamed.Sum(nil))
			w.WriteHeader(http.StatusOK)
		case "/api/v1/artifacts/commits":
			_ = json.NewEncoder(w).Encode(map[string]any{"artifact": map[string]any{"artifactId": ref.ArtifactID, "committedAt": "2026-07-24T12:00:01Z", "contentType": "application/gzip", "expiresAt": nil, "kind": ref.Kind, "producer": ref.Producer, "sha256": sha, "sizeBytes": len(contents), "state": "committed", "verification": "server-sha256"}, "artifactRef": ref})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	path := filepath.Join(t.TempDir(), "run.tar.gz")
	if err := os.WriteFile(path, contents, 0o600); err != nil {
		t.Fatal(err)
	}
	receipt, err := NewClient(server.Client()).Publish(context.Background(), path, Options{ContentType: "application/gzip", Kind: "cairntrace.run", Producer: producer, RunIndex: runIndex, ServiceURL: server.URL, Token: testPublisherToken})
	if err != nil {
		t.Fatal(err)
	}
	if uploadedBytes != int64(len(contents)) || uploadedSHA != sha {
		t.Fatalf("direct upload did not stream the exact hashed bytes")
	}
	if receipt.SizeBytes != int64(len(contents)) || receipt.SHA256 != sha {
		t.Fatalf("unexpected receipt: %#v", receipt)
	}
}

func TestLoadRunIndexRejectsUnknownTopLevelFields(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "run-index.json")
	data := []byte(`{"$schema":"urn:filecheap.dev:run-index:v1","version":1,"detector":{},"run":{},"health":{},"counts":{},"outcomes":[],"evidence":[],"rawLog":"secret"}`)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadRunIndex(path); err == nil || !strings.Contains(err.Error(), "unknown field") {
		t.Fatalf("expected a strict sidecar error, got %v", err)
	}
}

func TestMaxBytesMatchesThePlatformCeiling(t *testing.T) {
	t.Parallel()
	if MaxBytes != 64*1024*1024 {
		t.Fatalf("MaxBytes must track the 64 MiB platform ceiling, got %d", MaxBytes)
	}
	if got := transferTimeoutFor(MaxBytes); got <= transferBaseTimeout || got > maxTransferTimeout {
		t.Fatalf("transfer deadline for a ceiling-sized artifact is not bounded: %s", got)
	}
}

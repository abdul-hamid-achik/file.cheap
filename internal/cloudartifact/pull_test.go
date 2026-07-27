package cloudartifact

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/abdul-hamid-achik/file.cheap/internal/artifactref"
)

const testDeviceToken = "fcheap_device_ddddddddddddddddddddddddddddddddddddddddddd"

func TestPullStreamsAndInstallsVerifiedArtifact(t *testing.T) {
	t.Parallel()
	contents := []byte("verified private artifact")
	server, ref := downloadServer(t, contents, contents, nil)
	defer server.Close()
	destination := filepath.Join(t.TempDir(), "artifact.zst")

	result, err := NewClient(server.Client()).Pull(context.Background(), Options{
		ArtifactID:  ref.ArtifactID,
		Destination: destination,
		ServiceURL:  server.URL,
		Token:       testDeviceToken,
	})
	if err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(destination)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(contents) {
		t.Fatal("pulled bytes changed")
	}
	if result.ArtifactRef.URI != ref.URI || result.OutputPath != destination ||
		result.Verification != "server-sha256" {
		t.Fatalf("unexpected pull result: %#v", result)
	}
	info, err := os.Stat(destination)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("pull mode = %o, want 600", info.Mode().Perm())
	}
}

func TestPullRejectsDigestMismatchWithoutDestination(t *testing.T) {
	t.Parallel()
	declared := []byte("expected")
	server, ref := downloadServer(t, declared, []byte("tampered"), nil)
	defer server.Close()
	destination := filepath.Join(t.TempDir(), "artifact.bin")

	_, err := NewClient(server.Client()).Pull(context.Background(), Options{
		ArtifactID: ref.ArtifactID, Destination: destination,
		ServiceURL: server.URL, Token: testDeviceToken,
	})
	if err == nil || !strings.Contains(err.Error(), "SHA-256") {
		t.Fatalf("expected verification error, got %v", err)
	}
	if _, statErr := os.Stat(destination); !os.IsNotExist(statErr) {
		t.Fatalf("unverified destination exists: %v", statErr)
	}
}

func TestPullRefusesOverwriteBeforeRequestingGrant(t *testing.T) {
	t.Parallel()
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		http.Error(w, "unexpected", http.StatusInternalServerError)
	}))
	defer server.Close()
	destination := filepath.Join(t.TempDir(), "existing")
	if err := os.WriteFile(destination, []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}

	_, err := NewClient(server.Client()).Pull(context.Background(), Options{
		ArtifactID: "art_abcdefghijklmnop", Destination: destination,
		ServiceURL: server.URL, Token: testDeviceToken,
	})
	if err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("expected overwrite rejection, got %v", err)
	}
	if requests.Load() != 0 {
		t.Fatalf("requested a grant before rejecting destination")
	}
}

func TestPullRejectsRedirectWithoutLeakingSignedURL(t *testing.T) {
	t.Parallel()
	contents := []byte("redirected")
	server, ref := downloadServer(t, contents, contents, func(w http.ResponseWriter) {
		w.Header().Set("Location", "/elsewhere?secret=do-not-print")
		w.WriteHeader(http.StatusFound)
	})
	defer server.Close()

	_, err := NewClient(server.Client()).Pull(context.Background(), Options{
		ArtifactID:  ref.ArtifactID,
		Destination: filepath.Join(t.TempDir(), "artifact"),
		ServiceURL:  server.URL,
		Token:       testDeviceToken,
	})
	if err == nil || !strings.Contains(err.Error(), "unexpected status 302") {
		t.Fatalf("expected redirect rejection, got %v", err)
	}
	if strings.Contains(err.Error(), "do-not-print") {
		t.Fatal("pull error leaked a signed URL")
	}
}

func TestPullRejectsCredentialBearingGrantHeader(t *testing.T) {
	t.Parallel()
	contents := []byte("header")
	server, ref := downloadServer(
		t,
		contents,
		contents,
		nil,
		map[string]string{"Authorization": "Bearer signed-secret"},
	)
	defer server.Close()

	_, err := NewClient(server.Client()).Pull(context.Background(), Options{
		ArtifactID:  ref.ArtifactID,
		Destination: filepath.Join(t.TempDir(), "artifact"),
		ServiceURL:  server.URL,
		Token:       testDeviceToken,
	})
	if err == nil || !strings.Contains(err.Error(), "unsafe download header") {
		t.Fatalf("expected header rejection, got %v", err)
	}
}

func TestMaxBytesMatchesPrivatePlatformCeiling(t *testing.T) {
	t.Parallel()
	if MaxBytes != 64*1024*1024 {
		t.Fatalf("MaxBytes = %d", MaxBytes)
	}
	if transferTimeoutFor(MaxBytes) <= transferBaseTimeout ||
		transferTimeoutFor(MaxBytes) > maxTransferTimeout {
		t.Fatal("ceiling-sized pull timeout is not bounded")
	}
}

func TestValidateGrantRejectsMismatchedCloudIdentity(t *testing.T) {
	t.Parallel()
	producer := artifactref.Producer{Tool: "glyphrun"}
	ref, err := artifactref.NewCloud(
		"private",
		"art_abcdefghijklmnop",
		"glyphrun.run",
		producer,
	)
	if err != nil {
		t.Fatal(err)
	}
	valid := serviceResponse{
		Artifact: artifactDetails{
			ArtifactID:   ref.ArtifactID,
			CommittedAt:  stringPointer("2026-07-26T17:00:00Z"),
			Kind:         ref.Kind,
			Producer:     producer,
			SHA256:       strings.Repeat("a", 64),
			SizeBytes:    1,
			State:        "committed",
			Verification: "server-sha256",
		},
		ArtifactRef: ref,
		Download: transferGrant{
			ExpiresAt: "2030-01-01T00:00:00Z",
			Headers:   map[string]string{},
			Method:    http.MethodGet,
			URL:       "https://example.com/private?opaque=grant",
		},
	}
	if err := validateGrant(valid, ref.ArtifactID); err != nil {
		t.Fatalf("valid grant: %v", err)
	}

	wrongVault := valid
	wrongVault.ArtifactRef.URI = strings.Replace(
		wrongVault.ArtifactRef.URI,
		"/private/",
		"/another-owner/",
		1,
	)
	if err := validateGrant(wrongVault, ref.ArtifactID); err == nil {
		t.Fatal("accepted an artifact reference from a different cloud vault")
	}

	wrongProducer := valid
	producerCopy := *wrongProducer.ArtifactRef.Producer
	producerCopy.Tool = "cairntrace"
	wrongProducer.ArtifactRef.Producer = &producerCopy
	if err := validateGrant(wrongProducer, ref.ArtifactID); err == nil {
		t.Fatal("accepted an artifact reference with mismatched producer metadata")
	}
}

func stringPointer(value string) *string {
	return &value
}

type directHandler func(http.ResponseWriter)

func downloadServer(
	t *testing.T,
	declared []byte,
	downloaded []byte,
	direct directHandler,
	extraHeaders ...map[string]string,
) (*httptest.Server, artifactref.ArtifactRefV1) {
	t.Helper()
	producer := artifactref.Producer{
		Tool: "glyphrun", NativeSchema: "urn:glyphrun.dev:run:v1",
		NativeID: "run-1",
	}
	ref, err := artifactref.NewCloud(
		"private",
		"art_abcdefghijklmnop",
		"glyphrun.run",
		producer,
	)
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(declared)
	sha := hex.EncodeToString(digest[:])
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/console/artifacts/downloads":
			if r.Method != http.MethodPost ||
				r.Header.Get("Authorization") != "Bearer "+testDeviceToken {
				t.Errorf("unexpected control request")
				http.Error(w, "bad request", http.StatusBadRequest)
				return
			}
			var input map[string]string
			if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
				t.Error(err)
				return
			}
			headers := map[string]string{}
			if len(extraHeaders) > 0 {
				headers = extraHeaders[0]
			}
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"artifact": map[string]any{
					"artifactId": ref.ArtifactID, "committedAt": "2026-07-26T17:00:00Z",
					"contentType": "application/zstd", "expiresAt": nil,
					"kind": ref.Kind, "producer": producer, "sha256": sha,
					"sizeBytes": len(declared), "state": "committed",
					"verification": "server-sha256",
				},
				"artifactRef": ref,
				"download": map[string]any{
					"expiresAt": "2030-01-01T00:00:00Z", "headers": headers,
					"method": "GET", "url": server.URL + "/direct?grant=opaque",
				},
			})
		case "/direct":
			if direct != nil {
				direct(w)
				return
			}
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(downloaded)
		default:
			http.NotFound(w, r)
		}
	}))
	return server, ref
}

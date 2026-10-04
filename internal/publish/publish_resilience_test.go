package publish

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
	"sync"
	"sync/atomic"
	"testing"

	"github.com/abdul-hamid-achik/file.cheap/internal/artifactref"
)

type fixture struct {
	contents []byte
	sha      string
	path     string
	opts     Options
	ref      artifactref.ArtifactRefV1
}

func newFixture(t *testing.T) fixture {
	t.Helper()
	contents := []byte("resilience fixture bytes")
	digest := sha256.Sum256(contents)
	producer := artifactref.Producer{Tool: "chalupa", NativeSchema: "urn:chalupa:log-chunk:v1"}
	ref, err := artifactref.NewCloud("private", "art_abcdefghijklmnop", "chalupa.log-chunk", producer)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "artifact.zst")
	if err := os.WriteFile(path, contents, 0o600); err != nil {
		t.Fatal(err)
	}
	return fixture{
		contents: contents,
		sha:      hex.EncodeToString(digest[:]),
		path:     path,
		ref:      ref,
		opts:     Options{ContentType: "application/zstd", Kind: ref.Kind, Producer: producer, Token: testPublisherToken},
	}
}

func (f fixture) artifact(state string) map[string]any {
	return map[string]any{"artifactId": f.ref.ArtifactID, "committedAt": nil, "contentType": "application/zstd", "expiresAt": nil, "kind": f.ref.Kind, "producer": f.ref.Producer, "sha256": f.sha, "sizeBytes": len(f.contents), "state": state, "verification": "server-sha256", "futureArtifactField": "ignored"}
}

func (f fixture) planned(uploadURL string) map[string]any {
	return map[string]any{"artifact": f.artifact("planned"), "artifactRef": f.ref, "receipt": "123e4567-e89b-12d3-a456-426614174000", "upload": map[string]any{"expiresAt": "2030-01-01T00:00:00Z", "headers": map[string]string{"content-type": "application/zstd"}, "method": "PUT", "url": uploadURL}, "futureTopLevel": true}
}

func (f fixture) committed() map[string]any {
	return map[string]any{"artifact": f.artifact("committed"), "artifactRef": f.ref, "futureTopLevel": []int{1}}
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

// dropConnection simulates a lost response: the request reached the handler
// but the client sees a transport error.
func dropConnection(w http.ResponseWriter) {
	conn, _, err := w.(http.Hijacker).Hijack()
	if err == nil {
		_ = conn.Close()
	}
}

func TestPublishAcceptsAdditiveServerResponseFields(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/artifacts/plans":
			writeJSON(w, http.StatusCreated, f.planned(server.URL+"/direct"))
		case "/direct":
			w.WriteHeader(http.StatusOK)
		case "/api/v1/artifacts/commits":
			writeJSON(w, http.StatusOK, f.committed())
		}
	}))
	defer server.Close()
	f.opts.ServiceURL = server.URL
	receipt, err := NewClient(server.Client()).Publish(context.Background(), f.path, f.opts)
	if err != nil {
		t.Fatalf("additive response fields must not fail publish: %v", err)
	}
	if receipt.SHA256 != f.sha || receipt.ArtifactRef.ArtifactID != f.ref.ArtifactID {
		t.Fatalf("unexpected receipt: %#v", receipt)
	}
}

func TestPublishStillRejectsMalformedAndTrailingResponses(t *testing.T) {
	t.Parallel()
	for name, body := range map[string]string{
		"malformed": `{"artifact":`,
		"trailing":  `{"artifact":{}}{"second":true}`,
		"wrongtype": `{"artifact":{"sizeBytes":"big"}}`,
	} {
		f := newFixture(t)
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(body))
		}))
		f.opts.ServiceURL = server.URL
		_, err := NewClient(server.Client()).Publish(context.Background(), f.path, f.opts)
		server.Close()
		if err == nil || !strings.Contains(err.Error(), "decode artifact service response") && !strings.Contains(err.Error(), "trailing JSON") {
			t.Fatalf("%s response must fail decoding, got %v", name, err)
		}
	}
}

func TestPublishStillValidatesFieldsItUses(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		plan := f.planned("https://blob.example/x")
		plan["artifact"].(map[string]any)["sha256"] = strings.Repeat("0", 64)
		writeJSON(w, http.StatusCreated, plan)
	}))
	defer server.Close()
	f.opts.ServiceURL = server.URL
	if _, err := NewClient(server.Client()).Publish(context.Background(), f.path, f.opts); err == nil || !strings.Contains(err.Error(), "does not match the local file") {
		t.Fatalf("a plan for different bytes must still be refused, got %v", err)
	}
}

func TestPublishReportsProblemDetailOnFailure(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name        string
		status      int
		contentType string
		body        string
		want        string
		exact       string
		attempts    int32
	}{
		{name: "quota", status: http.StatusRequestEntityTooLarge, contentType: "application/problem+json", body: `{"type":"https://file.cheap/problems/quota","code":"producer_quota_exceeded","title":"Artifact exceeds the producer quota","detail":"producer 'chalupa' allows up to 10 bytes; this artifact declares 24 bytes.\u001b[0m"}`, want: "unexpected status 413 (producer_quota_exceeded): Artifact exceeds the producer quota: producer 'chalupa' allows up to 10 bytes", attempts: 1},
		{name: "non-json", status: http.StatusBadRequest, contentType: "text/html", body: "<html>do not echo " + testPublisherToken + "</html>", exact: "plan artifact publication: artifact service returned unexpected status 400", attempts: 1},
		{name: "server-error", status: http.StatusInternalServerError, contentType: "application/problem+json", body: `{"code":"internal_error","title":"Internal server error","detail":"The platform could not complete the request."}`, want: "unexpected status 500 (internal_error)", attempts: 2},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			f := newFixture(t)
			var attempts atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				attempts.Add(1)
				w.Header().Set("Content-Type", tc.contentType)
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.body))
			}))
			defer server.Close()
			f.opts.ServiceURL = server.URL
			_, err := NewClient(server.Client()).Publish(context.Background(), f.path, f.opts)
			if err == nil {
				t.Fatal("expected a failure")
			}
			if tc.exact != "" && err.Error() != tc.exact {
				t.Fatalf("error = %q, want %q", err.Error(), tc.exact)
			}
			if tc.want != "" && !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %q, want it to contain %q", err.Error(), tc.want)
			}
			if strings.Contains(err.Error(), testPublisherToken) || strings.ContainsRune(err.Error(), 0x1b) {
				t.Fatalf("error leaked a token or control character: %q", err.Error())
			}
			if got := attempts.Load(); got != tc.attempts {
				t.Fatalf("attempts = %d, want %d (5xx is transient and retried once, 4xx is not)", got, tc.attempts)
			}
			if isRetryable(err) != (tc.status >= 500) {
				t.Fatalf("retryable = %v for status %d", isRetryable(err), tc.status)
			}
		})
	}
}

func TestPublishReusesIdempotencyKeyAndResumesAfterLostCommit(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	var mu sync.Mutex
	var keys []string
	var planCalls, commitCalls, uploads int
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		switch r.URL.Path {
		case "/api/v1/artifacts/plans":
			var got planRequest
			_ = json.NewDecoder(r.Body).Decode(&got)
			keys = append(keys, got.IdempotencyKey)
			planCalls++
			if planCalls == 1 {
				dropConnection(w)
				return
			}
			writeJSON(w, http.StatusCreated, f.planned(server.URL+"/direct"))
		case "/direct":
			uploads++
			w.WriteHeader(http.StatusOK)
		case "/api/v1/artifacts/commits":
			commitCalls++
			if commitCalls == 1 {
				dropConnection(w)
				return
			}
			writeJSON(w, http.StatusOK, f.committed())
		}
	}))
	defer server.Close()
	f.opts.ServiceURL = server.URL
	receipt, err := NewClient(server.Client()).Publish(context.Background(), f.path, f.opts)
	if err != nil {
		t.Fatal(err)
	}
	if receipt.SHA256 != f.sha {
		t.Fatalf("unexpected receipt: %#v", receipt)
	}
	if len(keys) != 2 || keys[0] == "" || keys[0] != keys[1] {
		t.Fatalf("plan retry must reuse one idempotency key, got %v", keys)
	}
	if commitCalls != 2 || uploads != 1 {
		t.Fatalf("commits = %d (want 2), uploads = %d (want 1: the PUT is never retried)", commitCalls, uploads)
	}
}

func TestPublishAcceptsCommittedPlanReplay(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	var paths []string
	var mu sync.Mutex
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		paths = append(paths, r.URL.Path)
		mu.Unlock()
		if r.URL.Path != "/api/v1/artifacts/plans" {
			http.NotFound(w, r)
			return
		}
		writeJSON(w, http.StatusOK, f.committed())
	}))
	defer server.Close()
	f.opts.ServiceURL = server.URL
	receipt, err := NewClient(server.Client()).Publish(context.Background(), f.path, f.opts)
	if err != nil {
		t.Fatalf("a committed 200 replay is a successful publication: %v", err)
	}
	if receipt.ArtifactRef.ArtifactID != f.ref.ArtifactID || receipt.Verification != "server-sha256" {
		t.Fatalf("unexpected receipt: %#v", receipt)
	}
	if len(paths) != 1 {
		t.Fatalf("a replay must not upload or commit again, saw %v", paths)
	}
}

func TestPublishRejectsInconsistentReplayAndPlanStatuses(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	for name, handler := range map[string]func(http.ResponseWriter){
		"200 without a committed artifact": func(w http.ResponseWriter) { writeJSON(w, http.StatusOK, f.planned("https://blob.example/x")) },
		"201 for a committed artifact":     func(w http.ResponseWriter) { writeJSON(w, http.StatusCreated, f.committed()) },
		"replay of different bytes": func(w http.ResponseWriter) {
			other := f.committed()
			other["artifact"].(map[string]any)["sha256"] = strings.Repeat("1", 64)
			writeJSON(w, http.StatusOK, other)
		},
	} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { handler(w) }))
		opts := f.opts
		opts.ServiceURL = server.URL
		_, err := NewClient(server.Client()).Publish(context.Background(), f.path, opts)
		server.Close()
		if err == nil {
			t.Fatalf("%s must be refused", name)
		}
	}
}

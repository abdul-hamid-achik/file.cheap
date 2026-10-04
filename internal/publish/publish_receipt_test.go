package publish

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// committedWith returns a committed response whose artifact carries the given
// server timestamps (nil = JSON null, matching a service that has none).
func (f fixture) committedWith(committedAt, expiresAt any) map[string]any {
	response := f.committed()
	artifact := f.artifact("committed")
	artifact["committedAt"] = committedAt
	artifact["expiresAt"] = expiresAt
	response["artifact"] = artifact
	return response
}

func publishWithCommit(t *testing.T, f fixture, commit map[string]any) Receipt {
	t.Helper()
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/artifacts/plans":
			writeJSON(w, http.StatusCreated, f.planned(server.URL+"/direct"))
		case "/direct":
			w.WriteHeader(http.StatusOK)
		case "/api/v1/artifacts/commits":
			writeJSON(w, http.StatusOK, commit)
		}
	}))
	defer server.Close()
	f.opts.ServiceURL = server.URL
	client := NewClient(server.Client())
	client.now = func() time.Time { return time.Date(2026, 10, 4, 9, 0, 0, 0, time.UTC) }
	receipt, err := client.Publish(context.Background(), f.path, f.opts)
	if err != nil {
		t.Fatal(err)
	}
	return receipt
}

func TestReceiptCarriesServerCommittedAndExpiresAt(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	receipt := publishWithCommit(t, f, f.committedWith("2026-10-04T09:00:02.123Z", "2026-11-03T09:00:02Z"))
	if receipt.CommittedAt != "2026-10-04T09:00:02.123Z" || receipt.ExpiresAt != "2026-11-03T09:00:02Z" {
		t.Fatalf("server timestamps not carried verbatim: %#v", receipt)
	}
	// published_at stays the client clock, not the server's.
	if receipt.PublishedAt != "2026-10-04T09:00:00Z" {
		t.Fatalf("published_at = %q, want the client clock", receipt.PublishedAt)
	}
	raw, err := json.Marshal(receipt)
	if err != nil {
		t.Fatal(err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded["version"] != "filecheap-publish/1" || decoded["committed_at"] != receipt.CommittedAt || decoded["expires_at"] != receipt.ExpiresAt {
		t.Fatalf("receipt JSON = %s", raw)
	}
}

func TestReceiptOmitsAbsentOrInvalidServerTimestamps(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name                   string
		committedAt, expireAt  any
		wantCommitted, wantExp string
	}{
		{"both null", nil, nil, "", ""},
		{"expiry only", nil, "2026-11-03T09:00:02Z", "", "2026-11-03T09:00:02Z"},
		{"committed only", "2026-10-04T09:00:02Z", nil, "2026-10-04T09:00:02Z", ""},
		{"empty strings", "", "", "", ""},
		{"not rfc3339", "yesterday", "2026-11-03", "", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			f := newFixture(t)
			receipt := publishWithCommit(t, f, f.committedWith(tc.committedAt, tc.expireAt))
			if receipt.CommittedAt != tc.wantCommitted || receipt.ExpiresAt != tc.wantExp {
				t.Fatalf("receipt = %#v", receipt)
			}
			raw, _ := json.Marshal(receipt)
			if tc.wantCommitted == "" && strings.Contains(string(raw), "committed_at") {
				t.Fatalf("committed_at must be omitted when absent: %s", raw)
			}
			if tc.wantExp == "" && strings.Contains(string(raw), "expires_at") {
				t.Fatalf("expires_at must be omitted when absent: %s", raw)
			}
		})
	}
}

func TestReceiptFromCommittedPlanReplayCarriesServerTimestamps(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, f.committedWith("2026-10-04T08:00:00Z", "2026-10-11T08:00:00Z"))
	}))
	defer server.Close()
	f.opts.ServiceURL = server.URL
	receipt, err := NewClient(server.Client()).Publish(context.Background(), f.path, f.opts)
	if err != nil {
		t.Fatal(err)
	}
	if receipt.CommittedAt != "2026-10-04T08:00:00Z" || receipt.ExpiresAt != "2026-10-11T08:00:00Z" {
		t.Fatalf("replay receipt = %#v", receipt)
	}
}

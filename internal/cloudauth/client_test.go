package cloudauth

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestClientStartSessionAndLogout(t *testing.T) {
	t.Parallel()
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/console/auth/device-authorizations", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(Authorization{DeviceCode: "device", ExpiresIn: 600, Interval: 5, UserCode: "ABCD-EFGH", VerificationURI: "https://file.cheap/console/activate"})
	})
	mux.HandleFunc("GET /api/console/auth/session", func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer fcheap_device_token" {
			t.Fatalf("authorization header = %q", got)
		}
		_ = json.NewEncoder(w).Encode(Session{Email: "owner@example.com", UserID: "account"})
	})
	mux.HandleFunc("POST /api/console/auth/device-refresh", func(w http.ResponseWriter, r *http.Request) {
		var input map[string]string
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			t.Fatal(err)
		}
		if input["refreshToken"] != "fcheap_refresh_old" || input["nextRefreshToken"] != "fcheap_refresh_next" || input["rotationId"] != "rotation" {
			t.Fatalf("refresh input = %#v", input)
		}
		_ = json.NewEncoder(w).Encode(Token{
			AccessToken:      "fcheap_device_new",
			ExpiresIn:        900,
			RefreshExpiresIn: 2_592_000,
			RefreshToken:     input["nextRefreshToken"],
			TokenType:        "Bearer",
		})
	})
	mux.HandleFunc("DELETE /api/console/auth/device-session", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	client := NewClient(server.Client())
	authorization, err := client.Start(context.Background(), server.URL, "test CLI")
	if err != nil {
		t.Fatal(err)
	}
	if authorization.UserCode != "ABCD-EFGH" {
		t.Fatalf("user code = %q", authorization.UserCode)
	}
	session, err := client.Session(context.Background(), server.URL, "fcheap_device_token")
	if err != nil {
		t.Fatal(err)
	}
	if session.Email != "owner@example.com" {
		t.Fatalf("email = %q", session.Email)
	}
	refreshed, err := client.Refresh(context.Background(), server.URL, "fcheap_refresh_old", "fcheap_refresh_next", "rotation")
	if err != nil {
		t.Fatal(err)
	}
	if refreshed.RefreshToken != "fcheap_refresh_next" || refreshed.ExpiresIn != 900 {
		t.Fatalf("refreshed token = %#v", refreshed)
	}
	if err := client.Logout(context.Background(), server.URL, "fcheap_device_token"); err != nil {
		t.Fatal(err)
	}
}

func TestValidatedOriginRejectsPathsAndInsecureRemoteHTTP(t *testing.T) {
	t.Parallel()
	for _, value := range []string{"https://file.cheap/path", "http://file.cheap", "https://user:pass@file.cheap"} {
		if _, err := validatedOrigin(value); err == nil {
			t.Fatalf("validatedOrigin(%q) succeeded", value)
		}
	}
}

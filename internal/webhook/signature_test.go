package webhook

import (
	"strings"
	"testing"
	"time"
)

func TestGenerateSecret(t *testing.T) {
	secret, err := GenerateSecret()
	if err != nil {
		t.Fatalf("GenerateSecret() error = %v", err)
	}

	if !strings.HasPrefix(secret, "whsec_") {
		t.Errorf("GenerateSecret() = %v, want prefix whsec_", secret)
	}

	if len(secret) != 70 {
		t.Errorf("GenerateSecret() len = %d, want 70", len(secret))
	}
}

func TestGenerateSignature(t *testing.T) {
	payload := []byte(`{"type":"test","data":{}}`)
	secret := "whsec_test_secret"
	timestamp := time.Unix(1234567890, 0)

	sig := GenerateSignature(payload, secret, timestamp)

	if !strings.HasPrefix(sig, "sha256=") {
		t.Errorf("GenerateSignature() = %v, want prefix sha256=", sig)
	}

	sig2 := GenerateSignature(payload, secret, timestamp)
	if sig != sig2 {
		t.Error("GenerateSignature() should be deterministic")
	}

	sig3 := GenerateSignature(payload, "different_secret", timestamp)
	if sig == sig3 {
		t.Error("GenerateSignature() should vary with secret")
	}
}

func TestVerifySignature(t *testing.T) {
	payload := []byte(`{"type":"test","data":{}}`)
	secret := "whsec_test_secret"
	timestamp := time.Now()

	sig := GenerateSignature(payload, secret, timestamp)

	tests := []struct {
		name      string
		payload   []byte
		signature string
		secret    string
		timestamp time.Time
		tolerance time.Duration
		want      bool
	}{
		{
			name:      "valid signature",
			payload:   payload,
			signature: sig,
			secret:    secret,
			timestamp: timestamp,
			tolerance: 5 * time.Minute,
			want:      true,
		},
		{
			name:      "wrong signature",
			payload:   payload,
			signature: "sha256=invalid",
			secret:    secret,
			timestamp: timestamp,
			tolerance: 5 * time.Minute,
			want:      false,
		},
		{
			name:      "expired timestamp",
			payload:   payload,
			signature: sig,
			secret:    secret,
			timestamp: time.Now().Add(-10 * time.Minute),
			tolerance: 5 * time.Minute,
			want:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := VerifySignature(tt.payload, tt.signature, tt.secret, tt.timestamp, tt.tolerance)
			if got != tt.want {
				t.Errorf("VerifySignature() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestParseSignatureHeader(t *testing.T) {
	timestamp := time.Unix(1234567890, 0)
	sig := "sha256=abc123"
	header := BuildSignatureHeader(sig, timestamp)

	parsedSig, parsedTS, err := ParseSignatureHeader(header)
	if err != nil {
		t.Fatalf("ParseSignatureHeader() error = %v", err)
	}

	if parsedSig != sig {
		t.Errorf("ParseSignatureHeader() signature = %v, want %v", parsedSig, sig)
	}

	if !parsedTS.Equal(timestamp) {
		t.Errorf("ParseSignatureHeader() timestamp = %v, want %v", parsedTS, timestamp)
	}
}

func TestBuildSignatureHeader(t *testing.T) {
	sig := "sha256=abc123"
	timestamp := time.Unix(1234567890, 0)

	header := BuildSignatureHeader(sig, timestamp)

	if !strings.Contains(header, "t=1234567890") {
		t.Errorf("BuildSignatureHeader() = %v, should contain t=1234567890", header)
	}

	if !strings.Contains(header, "v1=abc123") {
		t.Errorf("BuildSignatureHeader() = %v, should contain v1=abc123", header)
	}
}

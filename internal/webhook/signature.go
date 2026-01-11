package webhook

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"
	"time"
)

func GenerateSecret() (string, error) {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return "whsec_" + hex.EncodeToString(bytes), nil
}

func GenerateSignature(payload []byte, secret string, timestamp time.Time) string {
	signedPayload := fmt.Sprintf("%d.%s", timestamp.Unix(), string(payload))
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(signedPayload))
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}

func VerifySignature(payload []byte, signature, secret string, timestamp time.Time, tolerance time.Duration) bool {
	if time.Since(timestamp) > tolerance {
		return false
	}

	expected := GenerateSignature(payload, secret, timestamp)
	return hmac.Equal([]byte(signature), []byte(expected))
}

func ParseSignatureHeader(header string) (signature string, timestamp time.Time, err error) {
	parts := strings.Split(header, ",")
	var ts int64

	for _, part := range parts {
		part = strings.TrimSpace(part)
		if val, ok := strings.CutPrefix(part, "t="); ok {
			ts, err = strconv.ParseInt(val, 10, 64)
			if err != nil {
				return "", time.Time{}, fmt.Errorf("invalid timestamp: %w", err)
			}
			timestamp = time.Unix(ts, 0)
		} else if val, ok := strings.CutPrefix(part, "v1="); ok {
			signature = "sha256=" + val
		}
	}

	if signature == "" {
		return "", time.Time{}, fmt.Errorf("signature not found")
	}
	if ts == 0 {
		return "", time.Time{}, fmt.Errorf("timestamp not found")
	}

	return signature, timestamp, nil
}

func BuildSignatureHeader(signature string, timestamp time.Time) string {
	sig := strings.TrimPrefix(signature, "sha256=")
	return fmt.Sprintf("t=%d,v1=%s", timestamp.Unix(), sig)
}

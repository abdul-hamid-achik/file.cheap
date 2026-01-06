package auth

import (
	"strings"
	"testing"
)

func TestHashPassword(t *testing.T) {
	tests := []struct {
		name     string
		password string
		wantErr  bool
	}{
		{"valid password", "mysecretpassword", false},
		{"short password", "short", false}, // bcrypt doesn't care about length
		{"long password", strings.Repeat("a", 72), false},
		{"empty password", "", false},
		{"unicode password", "пароль123", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hash, err := HashPassword(tt.password)
			if (err != nil) != tt.wantErr {
				t.Errorf("HashPassword() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && hash == "" {
				t.Error("HashPassword() returned empty hash")
			}
			if !tt.wantErr && hash == tt.password {
				t.Error("HashPassword() returned unhashed password")
			}
		})
	}
}

func TestCheckPassword(t *testing.T) {
	password := "mysecretpassword"
	hash, err := HashPassword(password)
	if err != nil {
		t.Fatalf("HashPassword() error = %v", err)
	}

	tests := []struct {
		name     string
		password string
		hash     string
		wantErr  bool
	}{
		{"correct password", password, hash, false},
		{"wrong password", "wrongpassword", hash, true},
		{"empty password", "", hash, true},
		{"invalid hash", password, "invalid", true},
		{"empty hash", password, "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := CheckPassword(tt.password, tt.hash)
			if (err != nil) != tt.wantErr {
				t.Errorf("CheckPassword() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestGenerateToken(t *testing.T) {
	token1, hash1, err := GenerateToken()
	if err != nil {
		t.Fatalf("GenerateToken() error = %v", err)
	}

	if token1 == "" {
		t.Error("GenerateToken() returned empty token")
	}
	if hash1 == "" {
		t.Error("GenerateToken() returned empty hash")
	}
	if token1 == hash1 {
		t.Error("GenerateToken() token and hash should be different")
	}

	// Test uniqueness
	token2, hash2, err := GenerateToken()
	if err != nil {
		t.Fatalf("GenerateToken() second call error = %v", err)
	}

	if token1 == token2 {
		t.Error("GenerateToken() should generate unique tokens")
	}
	if hash1 == hash2 {
		t.Error("GenerateToken() should generate unique hashes")
	}
}

func TestHashToken(t *testing.T) {
	token := "mysecrettoken"
	hash1 := HashToken(token)
	hash2 := HashToken(token)

	if hash1 == "" {
		t.Error("HashToken() returned empty hash")
	}
	if hash1 == token {
		t.Error("HashToken() returned unhashed token")
	}
	if hash1 != hash2 {
		t.Error("HashToken() should be deterministic")
	}

	// Different tokens should have different hashes
	hash3 := HashToken("differenttoken")
	if hash1 == hash3 {
		t.Error("HashToken() different tokens should have different hashes")
	}
}

func TestValidatePassword(t *testing.T) {
	tests := []struct {
		name     string
		password string
		wantErr  bool
	}{
		{"valid 8 chars", "12345678", false},
		{"valid 20 chars", "12345678901234567890", false},
		{"too short", "1234567", true},
		{"empty", "", true},
		{"max length", strings.Repeat("a", 128), false},
		{"too long", strings.Repeat("a", 129), true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidatePassword(tt.password)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidatePassword() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func BenchmarkHashPassword(b *testing.B) {
	password := "benchmarkpassword"
	for i := 0; i < b.N; i++ {
		HashPassword(password)
	}
}

func BenchmarkCheckPassword(b *testing.B) {
	password := "benchmarkpassword"
	hash, _ := HashPassword(password)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		CheckPassword(password, hash)
	}
}

func BenchmarkGenerateToken(b *testing.B) {
	for i := 0; i < b.N; i++ {
		GenerateToken()
	}
}

func BenchmarkHashToken(b *testing.B) {
	token := "benchmarktoken"
	for i := 0; i < b.N; i++ {
		HashToken(token)
	}
}

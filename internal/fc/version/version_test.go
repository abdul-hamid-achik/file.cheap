package version

import (
	"strings"
	"testing"
)

func TestFull(t *testing.T) {
	result := Full()
	if !strings.Contains(result, Version) {
		t.Errorf("Full() should contain Version, got %s", result)
	}
	if !strings.Contains(result, Commit) {
		t.Errorf("Full() should contain Commit, got %s", result)
	}
}

func TestShort(t *testing.T) {
	result := Short()
	if result != Version {
		t.Errorf("Short() = %s, want %s", result, Version)
	}
}

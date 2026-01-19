package api

import (
	"path/filepath"
	"strings"
)

// allowedMIMETypes defines the MIME types permitted for upload.
// This whitelist prevents uploading of potentially dangerous file types.
var allowedMIMETypes = map[string]bool{
	// Images
	"image/jpeg":    true,
	"image/png":     true,
	"image/gif":     true,
	"image/webp":    true,
	"image/svg+xml": true,
	"image/bmp":     true,
	"image/tiff":    true,
	"image/heic":    true,
	"image/heif":    true,
	"image/avif":    true,

	// Documents
	"application/pdf": true,

	// Videos
	"video/mp4":        true,
	"video/webm":       true,
	"video/quicktime":  true,
	"video/x-msvideo":  true,
	"video/x-matroska": true,
	"video/ogg":        true,
	"video/mpeg":       true,

	// Audio
	"audio/mpeg":  true,
	"audio/wav":   true,
	"audio/ogg":   true,
	"audio/flac":  true,
	"audio/aac":   true,
	"audio/webm":  true,
	"audio/x-m4a": true,

	// Fallback for unknown types (be cautious)
	"application/octet-stream": true,
}

// blockedExtensions defines file extensions that should never be accepted,
// regardless of MIME type, to prevent execution of malicious files.
var blockedExtensions = map[string]bool{
	".exe":   true,
	".bat":   true,
	".cmd":   true,
	".com":   true,
	".msi":   true,
	".scr":   true,
	".pif":   true,
	".sh":    true,
	".bash":  true,
	".zsh":   true,
	".ps1":   true,
	".vbs":   true,
	".vbe":   true,
	".js":    true,
	".jse":   true,
	".wsf":   true,
	".wsh":   true,
	".jar":   true,
	".php":   true,
	".asp":   true,
	".aspx":  true,
	".jsp":   true,
	".cgi":   true,
	".pl":    true,
	".py":    true,
	".rb":    true,
	".dll":   true,
	".so":    true,
	".dylib": true,
}

// IsAllowedMIMEType checks if a MIME type is in the allowed list.
func IsAllowedMIMEType(mimeType string) bool {
	// Normalize MIME type (remove parameters like charset)
	if idx := strings.Index(mimeType, ";"); idx != -1 {
		mimeType = strings.TrimSpace(mimeType[:idx])
	}
	mimeType = strings.ToLower(mimeType)
	return allowedMIMETypes[mimeType]
}

// IsBlockedExtension checks if a file extension is in the blocked list.
func IsBlockedExtension(filename string) bool {
	ext := strings.ToLower(filepath.Ext(filename))
	return blockedExtensions[ext]
}

// SanitizeFilename removes potentially dangerous characters and path components
// from a filename to prevent path traversal attacks.
func SanitizeFilename(filename string) string {
	// Get only the base name, removing any directory components
	filename = filepath.Base(filename)

	// Handle Windows-style paths that might slip through
	if idx := strings.LastIndex(filename, "\\"); idx != -1 {
		filename = filename[idx+1:]
	}

	// Remove null bytes and other control characters
	var sanitized strings.Builder
	for _, r := range filename {
		if r >= 32 && r != 127 && r != '/' && r != '\\' && r != ':' && r != '*' && r != '?' && r != '"' && r != '<' && r != '>' && r != '|' {
			sanitized.WriteRune(r)
		}
	}

	result := sanitized.String()

	// Prevent empty filenames
	if result == "" || result == "." || result == ".." {
		return "unnamed_file"
	}

	// Remove leading/trailing dots and spaces
	result = strings.Trim(result, ". ")

	// Limit filename length to prevent issues with filesystems
	if len(result) > 255 {
		ext := filepath.Ext(result)
		name := strings.TrimSuffix(result, ext)
		maxNameLen := 255 - len(ext)
		if maxNameLen > 0 && len(name) > maxNameLen {
			name = name[:maxNameLen]
		}
		result = name + ext
	}

	if result == "" {
		return "unnamed_file"
	}

	return result
}

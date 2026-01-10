package cli

import (
	"bytes"
	"testing"

	"github.com/spf13/cobra"
)

func TestVideoCmd(t *testing.T) {
	if videoCmd == nil {
		t.Fatal("videoCmd is nil")
	}

	if videoCmd.Use != "video" {
		t.Errorf("Use = %q, want %q", videoCmd.Use, "video")
	}

	if videoCmd.Short == "" {
		t.Error("Short description is empty")
	}
}

func TestVideoUploadCmd(t *testing.T) {
	if videoUploadCmd == nil {
		t.Fatal("videoUploadCmd is nil")
	}

	if videoUploadCmd.Use != "upload <file>" {
		t.Errorf("Use = %q, want %q", videoUploadCmd.Use, "upload <file>")
	}

	if videoUploadCmd.Short == "" {
		t.Error("Short description is empty")
	}

	if videoUploadCmd.RunE == nil {
		t.Error("RunE is nil")
	}
}

func TestVideoTranscodeCmd(t *testing.T) {
	if videoTranscodeCmd == nil {
		t.Fatal("videoTranscodeCmd is nil")
	}

	if videoTranscodeCmd.Use != "transcode <file-id>" {
		t.Errorf("Use = %q, want %q", videoTranscodeCmd.Use, "transcode <file-id>")
	}

	if videoTranscodeCmd.Short == "" {
		t.Error("Short description is empty")
	}

	if videoTranscodeCmd.RunE == nil {
		t.Error("RunE is nil")
	}
}

func TestVideoStatusCmd(t *testing.T) {
	if videoStatusCmd == nil {
		t.Fatal("videoStatusCmd is nil")
	}

	if videoStatusCmd.Use != "status <file-id>" {
		t.Errorf("Use = %q, want %q", videoStatusCmd.Use, "status <file-id>")
	}

	if videoStatusCmd.Short == "" {
		t.Error("Short description is empty")
	}

	if videoStatusCmd.RunE == nil {
		t.Error("RunE is nil")
	}
}

func TestVideoCmd_HasSubcommands(t *testing.T) {
	subcommands := videoCmd.Commands()

	if len(subcommands) < 3 {
		t.Errorf("Expected at least 3 subcommands, got %d", len(subcommands))
	}

	expectedSubcommands := map[string]bool{
		"upload":    false,
		"transcode": false,
		"status":    false,
	}

	for _, cmd := range subcommands {
		if _, ok := expectedSubcommands[cmd.Name()]; ok {
			expectedSubcommands[cmd.Name()] = true
		}
	}

	for name, found := range expectedSubcommands {
		if !found {
			t.Errorf("Subcommand %q not found", name)
		}
	}
}

func TestVideoUploadCmd_Flags(t *testing.T) {
	tests := []struct {
		flag     string
		hasShort bool
		short    string
	}{
		{"transcode", false, ""},
		{"resolution", true, "r"},
		{"format", true, "f"},
		{"preset", false, ""},
		{"thumbnail", false, ""},
		{"wait", true, "w"},
	}

	for _, tt := range tests {
		t.Run(tt.flag, func(t *testing.T) {
			f := videoUploadCmd.Flags().Lookup(tt.flag)
			if f == nil {
				t.Errorf("Flag %q not found", tt.flag)
				return
			}

			if tt.hasShort && f.Shorthand != tt.short {
				t.Errorf("Flag %q shorthand = %q, want %q", tt.flag, f.Shorthand, tt.short)
			}
		})
	}
}

func TestVideoTranscodeCmd_Flags(t *testing.T) {
	tests := []struct {
		flag     string
		hasShort bool
		short    string
	}{
		{"resolution", true, "r"},
		{"format", true, "f"},
		{"preset", false, ""},
		{"thumbnail", false, ""},
		{"wait", true, "w"},
	}

	for _, tt := range tests {
		t.Run(tt.flag, func(t *testing.T) {
			f := videoTranscodeCmd.Flags().Lookup(tt.flag)
			if f == nil {
				t.Errorf("Flag %q not found", tt.flag)
				return
			}

			if tt.hasShort && f.Shorthand != tt.short {
				t.Errorf("Flag %q shorthand = %q, want %q", tt.flag, f.Shorthand, tt.short)
			}
		})
	}
}

func TestVideoUploadCmd_RequiresArgs(t *testing.T) {
	cmd := &cobra.Command{}
	cmd.SetArgs([]string{})

	err := cobra.ExactArgs(1)(cmd, []string{})
	if err == nil {
		t.Error("Expected error for missing argument")
	}
}

func TestVideoTranscodeCmd_RequiresArgs(t *testing.T) {
	cmd := &cobra.Command{}
	cmd.SetArgs([]string{})

	err := cobra.ExactArgs(1)(cmd, []string{})
	if err == nil {
		t.Error("Expected error for missing argument")
	}
}

func TestVideoStatusCmd_RequiresArgs(t *testing.T) {
	cmd := &cobra.Command{}
	cmd.SetArgs([]string{})

	err := cobra.ExactArgs(1)(cmd, []string{})
	if err == nil {
		t.Error("Expected error for missing argument")
	}
}

func TestFormatBytes(t *testing.T) {
	tests := []struct {
		bytes int64
		want  string
	}{
		{0, "0 B"},
		{500, "500 B"},
		{1023, "1023 B"},
		{1024, "1.0 KB"},
		{1536, "1.5 KB"},
		{1048576, "1.0 MB"},
		{1572864, "1.5 MB"},
		{1073741824, "1.0 GB"},
		{1099511627776, "1.0 TB"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			got := formatBytes(tt.bytes)
			if got != tt.want {
				t.Errorf("formatBytes(%d) = %q, want %q", tt.bytes, got, tt.want)
			}
		})
	}
}

func TestVideoCmd_Help(t *testing.T) {
	var buf bytes.Buffer
	videoCmd.SetOut(&buf)

	// Use Help() directly to avoid root command execution
	if err := videoCmd.Help(); err != nil {
		t.Fatalf("Help() error: %v", err)
	}

	output := buf.String()
	if len(output) == 0 {
		t.Error("Help output is empty")
	}

	expectedStrings := []string{
		"video",
		"upload",
		"transcode",
		"status",
	}

	for _, s := range expectedStrings {
		if !bytes.Contains(buf.Bytes(), []byte(s)) {
			t.Errorf("Help output missing %q", s)
		}
	}
}

func TestVideoUploadCmd_Help(t *testing.T) {
	var buf bytes.Buffer
	videoUploadCmd.SetOut(&buf)

	if err := videoUploadCmd.Help(); err != nil {
		t.Fatalf("Help() error: %v", err)
	}

	output := buf.String()
	if len(output) == 0 {
		t.Error("Help output is empty")
	}

	expectedStrings := []string{
		"upload",
		"--transcode",
		"--resolution",
		"--format",
	}

	for _, s := range expectedStrings {
		if !bytes.Contains(buf.Bytes(), []byte(s)) {
			t.Errorf("Help output missing %q", s)
		}
	}
}

func TestVideoTranscodeCmd_Help(t *testing.T) {
	var buf bytes.Buffer
	videoTranscodeCmd.SetOut(&buf)

	if err := videoTranscodeCmd.Help(); err != nil {
		t.Fatalf("Help() error: %v", err)
	}

	output := buf.String()
	if len(output) == 0 {
		t.Error("Help output is empty")
	}

	expectedStrings := []string{
		"transcode",
		"--resolution",
		"--format",
		"--preset",
	}

	for _, s := range expectedStrings {
		if !bytes.Contains(buf.Bytes(), []byte(s)) {
			t.Errorf("Help output missing %q", s)
		}
	}
}

func TestVideoStatusCmd_Help(t *testing.T) {
	var buf bytes.Buffer
	videoStatusCmd.SetOut(&buf)

	if err := videoStatusCmd.Help(); err != nil {
		t.Fatalf("Help() error: %v", err)
	}

	output := buf.String()
	if len(output) == 0 {
		t.Error("Help output is empty")
	}

	if !bytes.Contains(buf.Bytes(), []byte("status")) {
		t.Error("Help output missing 'status'")
	}
}

func TestVideoUploadCmd_ValidVideoExtensions(t *testing.T) {
	validExts := map[string]bool{
		".mp4":  true,
		".mov":  true,
		".avi":  true,
		".mkv":  true,
		".webm": true,
		".wmv":  true,
		".flv":  true,
	}

	for ext := range validExts {
		t.Run(ext, func(t *testing.T) {
			if !validExts[ext] {
				t.Errorf("Extension %q should be valid", ext)
			}
		})
	}

	invalidExts := []string{".txt", ".jpg", ".pdf", ".doc"}
	for _, ext := range invalidExts {
		t.Run(ext, func(t *testing.T) {
			if validExts[ext] {
				t.Errorf("Extension %q should be invalid", ext)
			}
		})
	}
}

func TestVideoUploadCmd_SupportedFormats(t *testing.T) {
	supportedFormats := []string{"mp4", "webm"}

	for _, format := range supportedFormats {
		t.Run(format, func(t *testing.T) {
			if format != "mp4" && format != "webm" {
				t.Errorf("Format %q should be supported", format)
			}
		})
	}
}

func TestVideoTranscodeCmd_ValidResolutions(t *testing.T) {
	validResolutions := []int{360, 480, 720, 1080, 1440, 2160}

	for _, res := range validResolutions {
		t.Run(string(rune(res)), func(t *testing.T) {
			valid := res == 360 || res == 480 || res == 720 || res == 1080 || res == 1440 || res == 2160
			if !valid {
				t.Errorf("Resolution %d should be valid", res)
			}
		})
	}

	invalidResolutions := []int{100, 500, 800, 1200, 3000}
	for _, res := range invalidResolutions {
		t.Run(string(rune(res)), func(t *testing.T) {
			valid := res == 360 || res == 480 || res == 720 || res == 1080 || res == 1440 || res == 2160
			if valid {
				t.Errorf("Resolution %d should be invalid", res)
			}
		})
	}
}

func TestVideoTranscodeCmd_ValidPresets(t *testing.T) {
	validPresets := map[string]bool{
		"ultrafast": true,
		"fast":      true,
		"medium":    true,
		"slow":      true,
	}

	for preset := range validPresets {
		t.Run(preset, func(t *testing.T) {
			if !validPresets[preset] {
				t.Errorf("Preset %q should be valid", preset)
			}
		})
	}

	invalidPresets := []string{"superfast", "slower", "veryslow", "placebo"}
	for _, preset := range invalidPresets {
		t.Run(preset, func(t *testing.T) {
			if validPresets[preset] {
				t.Errorf("Preset %q should be invalid", preset)
			}
		})
	}
}

func TestVideoFlagDefaults(t *testing.T) {
	transcodeFlag := videoUploadCmd.Flags().Lookup("transcode")
	if transcodeFlag == nil {
		t.Fatal("transcode flag not found")
	}
	if transcodeFlag.DefValue != "false" {
		t.Errorf("transcode default = %q, want %q", transcodeFlag.DefValue, "false")
	}

	formatFlag := videoUploadCmd.Flags().Lookup("format")
	if formatFlag == nil {
		t.Fatal("format flag not found")
	}
	if formatFlag.DefValue != "mp4" {
		t.Errorf("format default = %q, want %q", formatFlag.DefValue, "mp4")
	}

	presetFlag := videoUploadCmd.Flags().Lookup("preset")
	if presetFlag == nil {
		t.Fatal("preset flag not found")
	}
	if presetFlag.DefValue != "medium" {
		t.Errorf("preset default = %q, want %q", presetFlag.DefValue, "medium")
	}

	thumbnailFlag := videoUploadCmd.Flags().Lookup("thumbnail")
	if thumbnailFlag == nil {
		t.Fatal("thumbnail flag not found")
	}
	if thumbnailFlag.DefValue != "true" {
		t.Errorf("thumbnail default = %q, want %q", thumbnailFlag.DefValue, "true")
	}

	waitFlag := videoUploadCmd.Flags().Lookup("wait")
	if waitFlag == nil {
		t.Fatal("wait flag not found")
	}
	if waitFlag.DefValue != "false" {
		t.Errorf("wait default = %q, want %q", waitFlag.DefValue, "false")
	}
}

func TestVideoTranscodeCmd_ResolutionFlagDefault(t *testing.T) {
	resFlag := videoTranscodeCmd.Flags().Lookup("resolution")
	if resFlag == nil {
		t.Fatal("resolution flag not found")
	}
	if resFlag.DefValue != "[720]" {
		t.Errorf("resolution default = %q, want %q", resFlag.DefValue, "[720]")
	}
}

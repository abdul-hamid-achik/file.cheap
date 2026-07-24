package artifactref

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const fixtureID = "demo_20260723_184500.123456789_0123456789abcdef01234567"

func TestNewLocalDefaultsAndProducer(t *testing.T) {
	t.Run("generic", func(t *testing.T) {
		ref, err := NewLocal(fixtureID, "generic", LocalOptions{})
		if err != nil {
			t.Fatal(err)
		}
		if ref.Schema != SchemaURI || ref.Version != Version || ref.Provider != ProviderLocal {
			t.Fatalf("identity = %+v", ref)
		}
		if ref.URI != "fcheap://stash/"+fixtureID || ref.ArtifactID != fixtureID {
			t.Fatalf("local identity = %+v", ref)
		}
		if ref.Kind != "filecheap.stash" || ref.Producer != nil {
			t.Fatalf("defaults = %+v", ref)
		}
	})

	t.Run("detected bundle", func(t *testing.T) {
		ref, err := NewLocal(fixtureID, "vidtrace", LocalOptions{})
		if err != nil {
			t.Fatal(err)
		}
		if ref.Kind != "vidtrace.bundle" {
			t.Fatalf("kind = %q, want vidtrace.bundle", ref.Kind)
		}
	})

	t.Run("override and producer", func(t *testing.T) {
		ref, err := NewLocal(fixtureID, "generic", LocalOptions{
			Kind: "cairntrace.run",
			Producer: Producer{
				Tool:         "cairntrace",
				Version:      "1.8.0",
				NativeSchema: "urn:cairntrace.dev:run:v1",
				NativeID:     "run_01",
				Entrypoint:   "producer/run.json",
			},
		})
		if err != nil {
			t.Fatal(err)
		}
		if ref.Kind != "cairntrace.run" || ref.Producer == nil || ref.Producer.Tool != "cairntrace" {
			t.Fatalf("ref = %+v", ref)
		}
		data, err := json.Marshal(ref)
		if err != nil {
			t.Fatal(err)
		}
		for _, forbidden := range []string{`"integrity"`, `"web_url"`} {
			if strings.Contains(string(data), forbidden) {
				t.Fatalf("JSON contains forbidden field %s: %s", forbidden, data)
			}
		}
	})
}

func TestNewLocalRejectsInvalidData(t *testing.T) {
	tests := []struct {
		name string
		id   string
		opts LocalOptions
		want string
	}{
		{name: "colon id", id: "sha256:abc", want: ".artifact_id"},
		{name: "oversized id", id: strings.Repeat("a", MaxArtifactIDLength+1), want: ".artifact_id"},
		{name: "unsafe kind", id: fixtureID, opts: LocalOptions{Kind: "Report Run"}, want: ".kind"},
		{
			name: "producer without tool",
			id:   fixtureID,
			opts: LocalOptions{Producer: Producer{NativeID: "run_01"}},
			want: ".producer.tool",
		},
		{
			name: "unsafe entrypoint",
			id:   fixtureID,
			opts: LocalOptions{Producer: Producer{Tool: "cairntrace", Entrypoint: "../run.json"}},
			want: ".producer.entrypoint",
		},
		{
			name: "credentialed schema URI",
			id:   fixtureID,
			opts: LocalOptions{Producer: Producer{Tool: "tool", NativeSchema: "https://user:pass@example.com/schema"}},
			want: ".producer.native_schema",
		},
		{
			name: "schema URI query",
			id:   fixtureID,
			opts: LocalOptions{Producer: Producer{Tool: "tool", NativeSchema: "https://example.com/schema?token=secret"}},
			want: ".producer.native_schema",
		},
		{
			name: "executable schema URI",
			id:   fixtureID,
			opts: LocalOptions{Producer: Producer{Tool: "tool", NativeSchema: "javascript:alert(1)"}},
			want: ".producer.native_schema",
		},
		{
			name: "insecure schema URI",
			id:   fixtureID,
			opts: LocalOptions{Producer: Producer{Tool: "tool", NativeSchema: "http://example.com/schema"}},
			want: ".producer.native_schema",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewLocal(tt.id, "generic", tt.opts)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %v, want field %s", err, tt.want)
			}
			var validationErr *ValidationError
			if !errors.As(err, &validationErr) {
				t.Fatalf("error type = %T, want *ValidationError", err)
			}
		})
	}
}

func TestValidateInterchangeProviderVariants(t *testing.T) {
	tests := []ArtifactRefV1{
		{
			Schema:     SchemaURI,
			Version:    Version,
			Provider:   ProviderCloud,
			URI:        "fcheap://cloud/vaults/vlt_01/artifacts/art_01",
			ArtifactID: "art_01",
			Kind:       "cairntrace.run",
			WebURL:     "https://artifacts.example/artifacts/art_01",
		},
		{
			Schema:   SchemaURI,
			Version:  Version,
			Provider: ProviderLink,
			URI:      "https://artifacts.example.com/reports/report-01",
			Kind:     "chalupa.report",
		},
	}
	for _, ref := range tests {
		if err := ref.Validate(); err != nil {
			t.Fatalf("Validate(%s): %v", ref.Provider, err)
		}
	}

	invalid := []ArtifactRefV1{
		{
			Schema:     SchemaURI,
			Version:    Version,
			Provider:   ProviderCloud,
			URI:        "fcheap://cloud/vaults/vlt_01/artifacts/different",
			ArtifactID: "art_01",
			Kind:       "cairntrace.run",
		},
		{
			Schema:   SchemaURI,
			Version:  Version,
			Provider: ProviderLink,
			URI:      "https://user:secret@artifacts.example.com/report",
			Kind:     "chalupa.report",
		},
		{
			Schema:   SchemaURI,
			Version:  Version,
			Provider: ProviderLink,
			URI:      "https://artifacts.example.com/report?token=secret",
			Kind:     "chalupa.report",
		},
		{
			Schema:     SchemaURI,
			Version:    Version,
			Provider:   ProviderLocal,
			URI:        LocalURI(fixtureID),
			ArtifactID: fixtureID,
			Kind:       "filecheap.stash",
			WebURL:     "https://artifacts.example/artifacts/local",
		},
		{
			Schema:     SchemaURI,
			Version:    Version,
			Provider:   ProviderCloud,
			URI:        "fcheap://cloud/vaults/vlt_01/artifacts/art_01",
			ArtifactID: "art_01",
			Kind:       "cairntrace.run",
			WebURL:     "https://artifacts.example/artifacts/art_01?token=secret",
		},
		{
			Schema:   SchemaURI,
			Version:  Version,
			Provider: ProviderLink,
			URI:      "https://artifacts.example:99999/report",
			Kind:     "chalupa.report",
		},
	}
	for _, ref := range invalid {
		if err := ref.Validate(); err == nil {
			t.Fatalf("Validate accepted invalid %s ref: %+v", ref.Provider, ref)
		}
	}
}

func TestValidateStableHTTPURLPortBounds(t *testing.T) {
	for _, test := range []struct {
		url  string
		want bool
	}{
		{url: "https://artifacts.example:0/report", want: true},
		{url: "https://artifacts.example:00000/report", want: true},
		{url: "https://artifacts.example:65535/report", want: true},
		{url: "https://artifacts.example:65536/report", want: false},
		{url: "https://artifacts.example:99999/report", want: false},
	} {
		err := validateStableHTTPURL(".uri", test.url, false)
		if (err == nil) != test.want {
			t.Fatalf("validateStableHTTPURL(%q) error = %v, want accepted=%t", test.url, err, test.want)
		}
	}
}

func TestParseJSONGoldenFixtures(t *testing.T) {
	root := filepath.Join("..", "..", "contracts", "artifact-ref", "v1")
	valid, err := filepath.Glob(filepath.Join(root, "valid", "*.json"))
	if err != nil {
		t.Fatal(err)
	}
	if len(valid) < 4 {
		t.Fatalf("valid fixture count = %d, want at least 4", len(valid))
	}
	for _, name := range valid {
		t.Run("valid/"+filepath.Base(name), func(t *testing.T) {
			data, err := os.ReadFile(name)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := ParseJSON(data); err != nil {
				t.Fatalf("ParseJSON: %v", err)
			}
		})
	}

	invalid, err := filepath.Glob(filepath.Join(root, "invalid", "*.json"))
	if err != nil {
		t.Fatal(err)
	}
	if len(invalid) < 5 {
		t.Fatalf("invalid fixture count = %d, want at least 5", len(invalid))
	}
	for _, name := range invalid {
		t.Run("invalid/"+filepath.Base(name), func(t *testing.T) {
			data, err := os.ReadFile(name)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := ParseJSON(data); err == nil {
				t.Fatal("ParseJSON accepted invalid fixture")
			}
		})
	}
}

func TestPublishedSchemaIdentityAndStrictness(t *testing.T) {
	path := filepath.Join("..", "..", "contracts", "artifact-ref", "v1", "schema.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var schema struct {
		ID                   string `json:"$id"`
		Description          string `json:"description"`
		AdditionalProperties bool   `json:"additionalProperties"`
		Required             []string
		Properties           map[string]json.RawMessage
		OneOf                []json.RawMessage `json:"oneOf"`
	}
	if err := json.Unmarshal(data, &schema); err != nil {
		t.Fatalf("decode schema: %v", err)
	}
	if schema.ID != SchemaURI || schema.AdditionalProperties {
		t.Fatalf("schema identity/strictness = %+v", schema)
	}
	if len(schema.OneOf) != 3 {
		t.Fatalf("schema provider variants = %d, want 3", len(schema.OneOf))
	}
	for _, field := range []string{"$schema", "version", "provider", "uri", "artifact_id", "kind", "web_url"} {
		if _, ok := schema.Properties[field]; !ok {
			t.Errorf("schema properties missing %q", field)
		}
	}
	for _, forbidden := range []string{"integrity"} {
		if _, ok := schema.Properties[forbidden]; ok {
			t.Errorf("schema unexpectedly includes %q", forbidden)
		}
	}
	if !strings.Contains(schema.Description, "Integrity is intentionally absent") ||
		!strings.Contains(schema.Description, "legacy ContentHash") {
		t.Fatalf("schema description does not explain omitted integrity: %q", schema.Description)
	}
	var linkVariant struct {
		Properties map[string]json.RawMessage `json:"properties"`
	}
	if err := json.Unmarshal(schema.OneOf[2], &linkVariant); err != nil {
		t.Fatalf("decode link schema variant: %v", err)
	}
	for _, raw := range []json.RawMessage{schema.Properties["web_url"], linkVariant.Properties["uri"]} {
		if !strings.Contains(string(raw), "3[0-5]") || strings.Contains(string(raw), "[0-9]{1,5}") {
			t.Fatalf("schema port pattern is not aligned with 0..65535: %s", raw)
		}
	}
	var provider struct {
		Enum []string `json:"enum"`
	}
	if err := json.Unmarshal(schema.Properties["provider"], &provider); err != nil {
		t.Fatalf("decode provider schema: %v", err)
	}
	wantProviders := []string{ProviderLocal, ProviderCloud, ProviderLink}
	if strings.Join(provider.Enum, ",") != strings.Join(wantProviders, ",") {
		t.Fatalf("provider enum = %v, want %v", provider.Enum, wantProviders)
	}
}

func TestParseJSONRejectsUnknownAndTrailingValues(t *testing.T) {
	base, err := NewLocal(fixtureID, "generic", LocalOptions{})
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(base)
	if err != nil {
		t.Fatal(err)
	}
	withUnknown := strings.TrimSuffix(string(data), "}") + `,"web_url":"https://example.invalid/signed?token=secret"}`
	if _, err := ParseJSON([]byte(withUnknown)); err == nil {
		t.Fatal("ParseJSON accepted web_url on a local reference")
	}
	if _, err := ParseJSON(append(data, []byte("\n{}")...)); err == nil {
		t.Fatal("ParseJSON accepted a trailing JSON value")
	}

	base.URI = LocalURI("different")
	mismatched, err := json.Marshal(base)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ParseJSON(mismatched); err == nil || !strings.Contains(err.Error(), ".uri") {
		t.Fatalf("mismatched URI error = %v", err)
	}

	for name, body := range map[string]string{
		"null producer": `{
			"$schema":"urn:filecheap.dev:artifact-ref:v1",
			"version":1,
			"provider":"link",
			"uri":"https://artifacts.example.com/report",
			"kind":"chalupa.report",
			"producer":null
		}`,
		"empty link artifact id": `{
			"$schema":"urn:filecheap.dev:artifact-ref:v1",
			"version":1,
			"provider":"link",
			"uri":"https://artifacts.example.com/report",
			"artifact_id":"",
			"kind":"chalupa.report"
		}`,
		"empty producer version": `{
			"$schema":"urn:filecheap.dev:artifact-ref:v1",
			"version":1,
			"provider":"link",
			"uri":"https://artifacts.example.com/report",
			"kind":"chalupa.report",
			"producer":{"tool":"chalupa","version":""}
		}`,
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := ParseJSON([]byte(body)); err == nil {
				t.Fatal("ParseJSON accepted an explicitly empty or null optional field")
			}
		})
	}
}

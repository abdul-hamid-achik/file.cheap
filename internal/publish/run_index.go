package publish

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/abdul-hamid-achik/file.cheap/internal/artifactref"
)

const maxRunIndexBytes int64 = 12 * 1024

// LoadRunIndex reads one explicit metadata-only sidecar. It never opens or
// extracts the artifact being published; producers remain responsible for
// generating the bounded projection next to their archive.
func LoadRunIndex(filePath string) (json.RawMessage, error) {
	info, err := os.Lstat(filePath)
	if err != nil {
		return nil, fmt.Errorf("inspect run index: %w", err)
	}
	if !info.Mode().IsRegular() || info.Size() <= 0 || info.Size() > maxRunIndexBytes {
		return nil, fmt.Errorf("run index must be a regular JSON file between 1 and %d bytes", maxRunIndexBytes)
	}
	file, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("open run index: %w", err)
	}
	defer file.Close() //nolint:errcheck
	opened, err := file.Stat()
	if err != nil {
		return nil, fmt.Errorf("inspect opened run index: %w", err)
	}
	if !opened.Mode().IsRegular() || !os.SameFile(info, opened) {
		return nil, errors.New("run index changed while opening")
	}
	data, err := io.ReadAll(io.LimitReader(file, maxRunIndexBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read run index: %w", err)
	}
	if int64(len(data)) > maxRunIndexBytes {
		return nil, fmt.Errorf("run index grew beyond %d bytes", maxRunIndexBytes)
	}
	if err := validateRunIndexDocument(data); err != nil {
		return nil, err
	}
	return json.RawMessage(data), nil
}

func validateRunIndex(raw json.RawMessage, producer artifactref.Producer) error {
	if len(raw) == 0 {
		return nil
	}
	envelope, err := decodeRunIndexEnvelope(raw)
	if err != nil {
		return err
	}
	var detector struct {
		Name string `json:"name"`
	}
	var run struct {
		NativeID string `json:"nativeId"`
	}
	if json.Unmarshal(envelope.Detector, &detector) != nil || json.Unmarshal(envelope.Run, &run) != nil {
		return errors.New("run index detector and run identity must be JSON objects")
	}
	expectedDetector := ""
	switch producer.Tool {
	case "cairntrace":
		expectedDetector = "cairntrace-run"
	case "glyphrun":
		expectedDetector = "glyphrun-run"
	}
	if expectedDetector == "" || detector.Name != expectedDetector {
		return errors.New("run index detector must match the producer tool")
	}
	if producer.NativeSchema == "" || producer.NativeID == "" || run.NativeID != producer.NativeID {
		return errors.New("run index requires matching producer native schema and native ID")
	}
	return nil
}

func validateRunIndexDocument(raw []byte) error {
	_, err := decodeRunIndexEnvelope(raw)
	return err
}

type runIndexEnvelope struct {
	Schema   string          `json:"$schema"`
	Version  int             `json:"version"`
	Detector json.RawMessage `json:"detector"`
	Run      json.RawMessage `json:"run"`
	Health   json.RawMessage `json:"health"`
	Counts   json.RawMessage `json:"counts"`
	Outcomes json.RawMessage `json:"outcomes"`
	Evidence json.RawMessage `json:"evidence"`
}

func decodeRunIndexEnvelope(raw []byte) (runIndexEnvelope, error) {
	var envelope runIndexEnvelope
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&envelope); err != nil {
		return envelope, fmt.Errorf("decode run index: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return envelope, errors.New("run index contains trailing JSON")
	}
	if envelope.Schema != "urn:filecheap.dev:run-index:v1" || envelope.Version != 1 ||
		len(envelope.Detector) == 0 || len(envelope.Run) == 0 || len(envelope.Health) == 0 ||
		len(envelope.Counts) == 0 || len(envelope.Outcomes) == 0 || len(envelope.Evidence) == 0 {
		return envelope, errors.New("run index is not a complete RunIndexV1 document")
	}
	return envelope, nil
}

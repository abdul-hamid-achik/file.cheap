package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/abdul-hamid-achik/file.cheap/internal/artifactref"
	"github.com/abdul-hamid-achik/file.cheap/internal/publish"
	"github.com/spf13/cobra"
)

var (
	publishServiceURL      string
	publishContentType     string
	publishExpiresIn       time.Duration
	publishKind            string
	publishProducerTool    string
	publishProducerVersion string
	publishNativeSchema    string
	publishNativeID        string
	publishEntrypoint      string
)

var publishCmd = &cobra.Command{
	Use:   "publish <file>",
	Short: "Publish one bounded file to the private artifact service",
	Long: `Publish one regular file through the private file.cheap artifact service.

	The command hashes an in-memory bounded copy, requests a direct upload grant,
uploads the exact bytes, and commits a server-verified ArtifactRefV1. It never
saves, deletes, or changes the local input. It accepts only FILECHEAP_INGEST_TOKEN
for non-Vercel callers; the kind, producer tool, and native schema must match
that credential's server-side policy. Vercel, Blob, and database credentials
are rejected.`,
	Example: `  FILECHEAP_ARTIFACT_SERVICE_URL=https://file.cheap \
  FILECHEAP_INGEST_TOKEN='…' \
  fcheap publish ./run.tar.gz --kind cairntrace.run --producer-tool cairntrace \
    --native-schema urn:cairntrace.dev:run:v1 --json`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		filePath, err := filepath.Abs(args[0])
		if err != nil {
			return err
		}
		serviceURL := publishServiceURL
		if serviceURL == "" {
			serviceURL = os.Getenv("FILECHEAP_ARTIFACT_SERVICE_URL")
		}
		token := os.Getenv("FILECHEAP_INGEST_TOKEN")
		kind := publishKind
		if kind == "" {
			kind = "filecheap.artifact"
		}
		producer := artifactref.Producer{
			Tool:         publishProducerTool,
			Version:      publishProducerVersion,
			NativeSchema: publishNativeSchema,
			NativeID:     publishNativeID,
			Entrypoint:   publishEntrypoint,
		}
		receipt, err := publish.NewClient(nil).Publish(GetContext(), filePath, publish.Options{
			ContentType: publishContentType,
			ExpiresIn:   publishExpiresIn,
			Kind:        kind,
			Producer:    producer,
			ServiceURL:  serviceURL,
			Token:       token,
		})
		if err != nil {
			return err
		}
		if printer.IsJSON() {
			return printer.JSON(receipt)
		}
		printer.Success("Published private artifact: %s", receipt.ArtifactRef.ArtifactID)
		printer.KeyValue("URI", receipt.ArtifactRef.URI)
		printer.KeyValue("Kind", receipt.ArtifactRef.Kind)
		printer.KeyValue("SHA-256", receipt.SHA256)
		printer.KeyValue("Size", formatSize(receipt.SizeBytes))
		printer.KeyValue("Verification", receipt.Verification)
		return nil
	},
}

func init() {
	publishCmd.Flags().StringVar(&publishServiceURL, "service-url", "", "Private artifact service origin (default: FILECHEAP_ARTIFACT_SERVICE_URL)")
	publishCmd.Flags().StringVar(&publishContentType, "content-type", "application/octet-stream", "Content type for the bounded file")
	publishCmd.Flags().DurationVar(&publishExpiresIn, "expires-in", 0, "Delete the remote artifact after this duration (1m to 744h; default: retain)")
	publishCmd.Flags().StringVar(&publishKind, "kind", "", "Artifact kind (default: filecheap.artifact)")
	publishCmd.Flags().StringVar(&publishProducerTool, "producer-tool", "fcheap", "Tool that produced the artifact")
	publishCmd.Flags().StringVar(&publishProducerVersion, "producer-version", "", "Version of the producer tool")
	publishCmd.Flags().StringVar(&publishNativeSchema, "native-schema", "", "Absolute schema URI for the native artifact")
	publishCmd.Flags().StringVar(&publishNativeID, "native-id", "", "Producer-native artifact ID")
	publishCmd.Flags().StringVar(&publishEntrypoint, "entrypoint", "", "Safe relative descriptor path inside the artifact")
	publishCmd.PreRunE = func(cmd *cobra.Command, args []string) error {
		if strings.TrimSpace(os.Getenv("FILECHEAP_INGEST_TOKEN")) == "" {
			return fmt.Errorf("FILECHEAP_INGEST_TOKEN is required for fcheap publish")
		}
		return nil
	}
}

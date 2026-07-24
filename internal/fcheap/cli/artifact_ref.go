package cli

import (
	"fmt"

	"github.com/abdul-hamid-achik/file.cheap/internal/artifactref"
	"github.com/abdul-hamid-achik/file.cheap/internal/stash"
	"github.com/spf13/cobra"
)

var (
	artifactRefKind            string
	artifactRefProducerTool    string
	artifactRefProducerVersion string
	artifactRefNativeSchema    string
	artifactRefNativeID        string
	artifactRefEntrypoint      string
)

var artifactRefCmd = &cobra.Command{
	Use:   "artifact-ref <stash-id>",
	Short: "Emit a portable reference to a local stash",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		mgr, err := stash.NewManager(cfg.StashDir)
		if err != nil {
			return err
		}
		st, err := mgr.Info(GetContext(), args[0])
		if err != nil {
			return err
		}
		ref, err := artifactref.NewLocal(st.Manifest.ID, st.Manifest.BundleType, artifactref.LocalOptions{
			Kind: artifactRefKind,
			Producer: artifactref.Producer{
				Tool:         artifactRefProducerTool,
				Version:      artifactRefProducerVersion,
				NativeSchema: artifactRefNativeSchema,
				NativeID:     artifactRefNativeID,
				Entrypoint:   artifactRefEntrypoint,
			},
		})
		if err != nil {
			return err
		}
		if ref.Producer != nil {
			if err := mgr.ValidateArtifactEntrypoint(GetContext(), st, ref.Producer.Entrypoint); err != nil {
				return err
			}
		}

		if printer.IsJSON() {
			return printer.JSON(ref)
		}

		printer.Header("Artifact Reference")
		printer.KeyValue("Schema", ref.Schema)
		printer.KeyValue("Version", fmt.Sprintf("%d", ref.Version))
		printer.KeyValue("Provider", ref.Provider)
		printer.KeyValue("URI", ref.URI)
		printer.KeyValue("Artifact ID", ref.ArtifactID)
		printer.KeyValue("Kind", ref.Kind)
		if ref.Producer != nil {
			printer.Section("Producer")
			printer.KeyValue("Tool", ref.Producer.Tool)
			if ref.Producer.Version != "" {
				printer.KeyValue("Version", ref.Producer.Version)
			}
			if ref.Producer.NativeSchema != "" {
				printer.KeyValue("Native Schema", ref.Producer.NativeSchema)
			}
			if ref.Producer.NativeID != "" {
				printer.KeyValue("Native ID", ref.Producer.NativeID)
			}
			if ref.Producer.Entrypoint != "" {
				printer.KeyValue("Entrypoint", ref.Producer.Entrypoint)
			}
		}
		return nil
	},
}

func init() {
	artifactRefCmd.Flags().StringVar(&artifactRefKind, "kind", "", "Artifact kind override (default: derived from the stash bundle)")
	artifactRefCmd.Flags().StringVar(&artifactRefProducerTool, "producer-tool", "", "Tool that produced the native artifact")
	artifactRefCmd.Flags().StringVar(&artifactRefProducerVersion, "producer-version", "", "Version of the producer tool")
	artifactRefCmd.Flags().StringVar(&artifactRefNativeSchema, "native-schema", "", "Absolute schema URI for the native artifact")
	artifactRefCmd.Flags().StringVar(&artifactRefNativeID, "native-id", "", "Producer-native artifact ID")
	artifactRefCmd.Flags().StringVar(&artifactRefEntrypoint, "entrypoint", "", "Safe relative path to the native descriptor inside the stash")
}

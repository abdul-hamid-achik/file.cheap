package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

var shareCmd = &cobra.Command{
	Use:   "share [file-id]",
	Short: "Create a shareable link for a file",
	Long: `Create a shareable link for a file.

The link can be used by anyone to access the file, even without authentication.
You can optionally set an expiration time for the link.

Examples:
  fc share abc123                     # Create a share link (never expires)
  fc share abc123 --expires 24h       # Link expires in 24 hours
  fc share abc123 --expires 7d        # Link expires in 7 days
  fc share abc123 --expires 1h        # Link expires in 1 hour`,
	Args: cobra.ExactArgs(1),
	RunE: runShare,
}

var shareExpires string

func init() {
	shareCmd.Flags().StringVarP(&shareExpires, "expires", "e", "", "Expiration time (e.g., 1h, 24h, 7d)")
}

func runShare(cmd *cobra.Command, args []string) error {
	if err := requireAuth(); err != nil {
		return err
	}

	fileID := args[0]
	ctx := GetContext()

	if !jsonOutput {
		printer.Info("Creating share link for %s...", fileID)
	}

	result, err := apiClient.CreateShare(ctx, fileID, shareExpires)
	if err != nil {
		return fmt.Errorf("failed to create share link: %w", err)
	}

	if jsonOutput {
		return printer.JSON(map[string]interface{}{
			"id":         result.ID,
			"file_id":    fileID,
			"token":      result.Token,
			"share_url":  result.ShareURL,
			"expires_at": result.ExpiresAt,
		})
	}

	printer.Success("Share link created!")
	printer.Println()
	printer.Section("Share Details")
	printer.KeyValue("Share ID", result.ID)
	printer.KeyValue("URL", result.ShareURL)
	if result.ExpiresAt != nil {
		printer.KeyValue("Expires", result.ExpiresAt.Format("2006-01-02 15:04:05"))
	} else {
		printer.KeyValue("Expires", "Never")
	}
	printer.Println()
	printer.Info("Anyone with this URL can access the file.")

	return nil
}

package cli

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
)

var deleteCmd = &cobra.Command{
	Use:     "delete [file-id...]",
	Aliases: []string{"rm"},
	Short:   "Delete files",
	Long: `Delete files from file.cheap.

Examples:
  fc delete abc123              # Delete single file
  fc delete abc123 def456       # Delete multiple files
  fc delete abc123 --force      # Skip confirmation`,
	RunE: runDelete,
}

var deleteForce bool

func init() {
	deleteCmd.Flags().BoolVarP(&deleteForce, "force", "f", false, "Skip confirmation")
}

func runDelete(cmd *cobra.Command, args []string) error {
	if err := requireAuth(); err != nil {
		return err
	}

	if len(args) == 0 {
		return fmt.Errorf("no file IDs specified")
	}

	if !deleteForce && !jsonOutput {
		printer.Printf("Are you sure you want to delete %d file(s)? [y/N] ", len(args))
		reader := bufio.NewReader(os.Stdin)
		response, _ := reader.ReadString('\n')
		response = strings.TrimSpace(strings.ToLower(response))
		if response != "y" && response != "yes" {
			printer.Info("Cancelled")
			return nil
		}
	}

	ctx := GetContext()
	var successful, failed int
	var results []map[string]interface{}

	for _, fileID := range args {
		err := apiClient.DeleteFile(ctx, fileID)
		if err != nil {
			if !jsonOutput {
				printer.FileFailed(fileID, err)
			}
			results = append(results, map[string]interface{}{
				"id":    fileID,
				"error": err.Error(),
			})
			failed++
		} else {
			if !jsonOutput {
				printer.Success("Deleted %s", fileID)
			}
			results = append(results, map[string]interface{}{
				"id":      fileID,
				"deleted": true,
			})
			successful++
		}
	}

	if jsonOutput {
		return printer.JSON(map[string]interface{}{
			"results":    results,
			"total":      len(args),
			"successful": successful,
			"failed":     failed,
		})
	}

	printer.Summary(successful, failed)

	if failed > 0 {
		return fmt.Errorf("%d deletions failed", failed)
	}
	return nil
}

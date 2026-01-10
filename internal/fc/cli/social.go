package cli

import (
	"context"
	"fmt"
	"time"

	"github.com/abdul-hamid-achik/file.cheap/internal/fc/client"
	"github.com/abdul-hamid-achik/file.cheap/internal/fc/output"
	"github.com/spf13/cobra"
)

var socialCmd = &cobra.Command{
	Use:   "social [file-id or file]",
	Short: "Generate social media variants",
	Long: `Generate social media optimized images in one command.

Platforms:
  og                 1200x630  Open Graph (Facebook, LinkedIn)
  twitter            1200x675  Twitter/X cards
  instagram_square   1080x1080 Instagram feed
  instagram_portrait 1080x1350 Instagram portrait
  instagram_story    1080x1920 Instagram/TikTok stories

Examples:
  fc social hero.png                    # All platforms
  fc social abc123 --platforms=og,twitter
  fc social hero.png --title="My Post"`,
	RunE: runSocial,
}

var (
	socialPlatforms []string
	socialTitle     string
	socialWait      bool
)

var defaultSocialPlatforms = []string{
	"og",
	"twitter",
	"instagram_square",
	"instagram_portrait",
	"instagram_story",
}

func init() {
	socialCmd.Flags().StringSliceVar(&socialPlatforms, "platforms", nil, "Platforms to generate (og, twitter, instagram_square, instagram_portrait, instagram_story)")
	socialCmd.Flags().StringVar(&socialTitle, "title", "", "Title text overlay (for og images)")
	socialCmd.Flags().BoolVarP(&socialWait, "wait", "w", false, "Wait for processing to complete")
}

func runSocial(cmd *cobra.Command, args []string) error {
	if err := requireAuth(); err != nil {
		return err
	}

	if len(args) == 0 {
		return fmt.Errorf("no file specified")
	}

	platforms := socialPlatforms
	if len(platforms) == 0 {
		platforms = defaultSocialPlatforms
	}

	ctx := context.Background()
	input := args[0]

	var fileID string

	if isImageFile(input) {
		printer.Info("Uploading %s...", input)
		result, err := apiClient.Upload(ctx, input, nil, false)
		if err != nil {
			return fmt.Errorf("failed to upload: %w", err)
		}
		fileID = result.ID
		printer.Success("Uploaded as %s", fileID)
	} else {
		fileID = input
	}

	printer.Printf("Generating social variants for %s...\n", fileID)

	req := &client.TransformRequest{
		Presets: platforms,
	}

	if socialTitle != "" {
		printer.Info("Title overlay: %s (feature coming soon)", socialTitle)
	}

	resp, err := apiClient.Transform(ctx, fileID, req)
	if err != nil {
		return fmt.Errorf("failed to create social variants: %w", err)
	}

	if jsonOutput && !socialWait {
		return printer.JSON(map[string]interface{}{
			"file_id":   fileID,
			"platforms": platforms,
			"jobs":      resp.Jobs,
		})
	}

	if socialWait {
		spinner := output.NewSpinner("Processing variants...", quietMode)
		file, err := apiClient.WaitForFile(ctx, fileID, 2*time.Second, 5*time.Minute)
		spinner.Finish()

		if err != nil {
			printer.Warn("Timeout waiting for processing")
		} else if file.Status == "completed" {
			for _, variant := range file.Variants {
				for _, p := range platforms {
					if variant.VariantType == p {
						printer.Success("%s: %s/cdn/%s/%s/%s", p, cfg.BaseURL, fileID, p, "image.jpg")
					}
				}
			}
		}
	} else {
		for _, p := range platforms {
			printer.Info("%s: queued", p)
		}
	}

	if jsonOutput {
		file, _ := apiClient.GetFile(ctx, fileID)
		if file != nil {
			return printer.JSON(map[string]interface{}{
				"file_id":   fileID,
				"platforms": platforms,
				"status":    file.Status,
				"variants":  file.Variants,
			})
		}
	}

	printer.Println()
	printer.Printf("%d variants queued\n", len(platforms))

	if !socialWait {
		printer.Info("Use 'fc status %s' to check progress", fileID)
	}

	return nil
}

package mcp

import (
	"context"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func registerPrompts(srv *mcp.Server) {
	pathArg := &mcp.PromptArgument{
		Name:        "path",
		Description: "Path to the source file",
		Required:    true,
	}

	// optimize-for-web
	srv.AddPrompt(&mcp.Prompt{
		Name:        "optimize-for-web",
		Title:       "Optimize Image for Web",
		Description: "Optimize an image for web delivery: compress, convert to WebP, generate responsive sizes",
		Arguments:   []*mcp.PromptArgument{pathArg},
	}, func(ctx context.Context, req *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
		path := req.Params.Arguments["path"]
		return &mcp.GetPromptResult{
			Description: fmt.Sprintf("Optimize %s for web delivery", path),
			Messages: []*mcp.PromptMessage{
				{Role: "user", Content: &mcp.TextContent{Text: fmt.Sprintf(`I need to optimize the image at %s for web delivery. Please follow these steps:

1. Use fcheap_file_info to inspect the file and confirm it is a supported image format.
2. Use fcheap_optimize_image to compress the original with quality 85.
3. Use fcheap_convert_to_webp to create a WebP version with quality 85.
4. Use fcheap_apply_preset with preset "sm" (640px) to generate a mobile-friendly size.
5. Use fcheap_apply_preset with preset "md" (1024px) to generate a tablet-friendly size.
6. Use fcheap_apply_preset with preset "lg" (1440px) to generate a desktop-friendly size.
7. For each generated file, convert it to WebP using fcheap_convert_to_webp.

Report the file sizes before and after each operation so I can see the savings.`, path)}},
			},
		}, nil
	})

	// social-media-pack
	srv.AddPrompt(&mcp.Prompt{
		Name:        "social-media-pack",
		Title:       "Social Media Image Pack",
		Description: "Generate all social media image sizes from a source image",
		Arguments:   []*mcp.PromptArgument{pathArg},
	}, func(ctx context.Context, req *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
		path := req.Params.Arguments["path"]
		return &mcp.GetPromptResult{
			Description: fmt.Sprintf("Generate social media pack from %s", path),
			Messages: []*mcp.PromptMessage{
				{Role: "user", Content: &mcp.TextContent{Text: fmt.Sprintf(`I need to generate a complete social media image pack from %s. Please follow these steps:

1. Use fcheap_file_info to inspect the source image and confirm its dimensions.
2. Generate all social media sizes using fcheap_apply_preset:
   - "og" (1200x630) — Open Graph / Facebook sharing
   - "twitter" (1200x675) — Twitter/X card image
   - "instagram_square" (1080x1080) — Instagram feed post
   - "instagram_portrait" (1080x1350) — Instagram portrait post
   - "instagram_story" (1080x1920) — Instagram/Facebook story
3. For each generated image, also create a WebP version using fcheap_convert_to_webp for web embedding.
4. Summarize all generated files with their dimensions and file sizes.`, path)}},
			},
		}, nil
	})

	// responsive-image-set
	srv.AddPrompt(&mcp.Prompt{
		Name:        "responsive-image-set",
		Title:       "Responsive Image Set",
		Description: "Generate a complete responsive image set with WebP variants",
		Arguments:   []*mcp.PromptArgument{pathArg},
	}, func(ctx context.Context, req *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
		path := req.Params.Arguments["path"]
		return &mcp.GetPromptResult{
			Description: fmt.Sprintf("Generate responsive image set from %s", path),
			Messages: []*mcp.PromptMessage{
				{Role: "user", Content: &mcp.TextContent{Text: fmt.Sprintf(`I need to generate a complete responsive image set from %s for use in HTML <picture> and srcset attributes. Please follow these steps:

1. Use fcheap_file_info to inspect the source image and confirm its dimensions.
2. Generate responsive breakpoint sizes using fcheap_apply_preset:
   - "sm" (640px wide) — mobile devices
   - "md" (1024px wide) — tablets
   - "lg" (1440px wide) — laptops and desktops
   - "xl" (1920px wide) — large displays
3. For each generated size, create a WebP variant using fcheap_convert_to_webp with quality 85.
4. Summarize all generated files with their dimensions and file sizes.
5. Provide an HTML <picture> snippet that uses the generated files with appropriate media queries and WebP fallbacks. For example:

<picture>
  <source srcset="image_sm.webp" media="(max-width: 640px)" type="image/webp">
  <source srcset="image_sm.jpg" media="(max-width: 640px)">
  <source srcset="image_md.webp" media="(max-width: 1024px)" type="image/webp">
  <source srcset="image_md.jpg" media="(max-width: 1024px)">
  <source srcset="image_lg.webp" media="(max-width: 1440px)" type="image/webp">
  <source srcset="image_lg.jpg" media="(max-width: 1440px)">
  <source srcset="image_xl.webp" type="image/webp">
  <img src="image_xl.jpg" alt="" loading="lazy">
</picture>`, path)}},
			},
		}, nil
	})

	// video-for-web
	srv.AddPrompt(&mcp.Prompt{
		Name:        "video-for-web",
		Title:       "Prepare Video for Web",
		Description: "Prepare a video for web delivery",
		Arguments:   []*mcp.PromptArgument{pathArg},
	}, func(ctx context.Context, req *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
		path := req.Params.Arguments["path"]
		return &mcp.GetPromptResult{
			Description: fmt.Sprintf("Prepare %s for web delivery", path),
			Messages: []*mcp.PromptMessage{
				{Role: "user", Content: &mcp.TextContent{Text: fmt.Sprintf(`I need to prepare the video at %s for web delivery. Please follow these steps:

1. Use fcheap_list_capabilities to verify that ffmpeg is available. If it is not installed, stop and inform me that ffmpeg is required (install via "brew install ffmpeg" or "apt install ffmpeg").
2. Use fcheap_video_metadata to inspect the source video (duration, resolution, codecs, bitrate).
3. Use fcheap_video_thumbnail to extract a poster image from the video.
4. Use fcheap_transcode_video to create a web-optimized MP4:
   - If the source is already H.264 MP4 with reasonable bitrate (under 5 Mbps), you may skip this step.
   - Otherwise transcode to MP4 with format "mp4".
5. If the video is longer than 30 seconds, use fcheap_generate_hls to create an HLS adaptive streaming package for better playback on slow connections.
6. Summarize the results:
   - Original file: size, duration, resolution, codecs
   - Poster image: path and dimensions
   - Transcoded MP4: size and compression ratio
   - HLS package: segment count and directory path (if generated)
7. Provide an HTML <video> snippet for embedding:

<video controls preload="metadata" poster="thumbnail.jpg">
  <source src="video.mp4" type="video/mp4">
  Your browser does not support the video tag.
</video>`, path)}},
			},
		}, nil
	})
}

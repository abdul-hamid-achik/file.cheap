package cli

import (
	"fmt"
	"strings"

	"github.com/abdul-hamid-achik/file-processor/internal/fc/config"
	"github.com/spf13/cobra"
)

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Manage CLI configuration",
	Long:  `View and manage fc CLI configuration.`,
}

var configShowCmd = &cobra.Command{
	Use:   "show",
	Short: "Show current configuration",
	RunE:  runConfigShow,
}

var configSetCmd = &cobra.Command{
	Use:   "set [key] [value]",
	Short: "Set a configuration value",
	Long: `Set a configuration value.

Available keys:
  base_url           API base URL
  parallel           Default parallel uploads (1-20)
  default_transforms Default transforms to apply

Examples:
  fc config set base_url https://api.file.cheap
  fc config set parallel 8
  fc config set default_transforms webp,thumbnail`,
	Args: cobra.ExactArgs(2),
	RunE: runConfigSet,
}

var configPathCmd = &cobra.Command{
	Use:   "path",
	Short: "Show config file path",
	RunE:  runConfigPath,
}

var configPresetsCmd = &cobra.Command{
	Use:   "presets",
	Short: "List available presets",
	RunE:  runConfigPresets,
}

func init() {
	configCmd.AddCommand(configShowCmd)
	configCmd.AddCommand(configSetCmd)
	configCmd.AddCommand(configPathCmd)
	configCmd.AddCommand(configPresetsCmd)
}

func runConfigShow(cmd *cobra.Command, args []string) error {
	if jsonOutput {
		return printer.JSON(map[string]interface{}{
			"base_url":           cfg.BaseURL,
			"authenticated":      cfg.IsAuthenticated(),
			"parallel":           cfg.Parallel,
			"default_transforms": cfg.DefaultTransforms,
			"presets":            cfg.Presets,
		})
	}

	printer.Section("Configuration")
	printer.KeyValue("Base URL", cfg.BaseURL)
	printer.KeyValue("Authenticated", fmt.Sprintf("%v", cfg.IsAuthenticated()))
	printer.KeyValue("Parallel", fmt.Sprintf("%d", cfg.Parallel))

	if len(cfg.DefaultTransforms) > 0 {
		printer.KeyValue("Default Transforms", strings.Join(cfg.DefaultTransforms, ", "))
	}

	if len(cfg.Presets) > 0 {
		printer.Section("Custom Presets")
		for name, preset := range cfg.Presets {
			printer.Printf("  %s: %s\n", name, strings.Join(preset.Transforms, ", "))
		}
	}

	return nil
}

func runConfigSet(cmd *cobra.Command, args []string) error {
	key := args[0]
	value := args[1]

	switch key {
	case "base_url":
		cfg.BaseURL = value
	case "parallel":
		var p int
		if _, err := fmt.Sscanf(value, "%d", &p); err != nil {
			return fmt.Errorf("invalid parallel value: %s", value)
		}
		if p < 1 || p > 20 {
			return fmt.Errorf("parallel must be between 1 and 20")
		}
		cfg.Parallel = p
	case "default_transforms":
		cfg.DefaultTransforms = strings.Split(value, ",")
		for i, t := range cfg.DefaultTransforms {
			cfg.DefaultTransforms[i] = strings.TrimSpace(t)
		}
	default:
		return fmt.Errorf("unknown config key: %s", key)
	}

	if err := cfg.Save(); err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}

	printer.Success("Set %s = %s", key, value)
	return nil
}

func runConfigPath(cmd *cobra.Command, args []string) error {
	path, err := config.Path()
	if err != nil {
		return err
	}

	if jsonOutput {
		return printer.JSON(map[string]string{"path": path})
	}

	printer.Println(path)
	return nil
}

func runConfigPresets(cmd *cobra.Command, args []string) error {
	if jsonOutput {
		allPresets := make(map[string]config.Preset)
		for name, preset := range config.BuiltinPresets {
			allPresets[name] = preset
		}
		for name, preset := range cfg.Presets {
			allPresets[name] = preset
		}
		return printer.JSON(allPresets)
	}

	printer.Section("Built-in Presets")
	for name, preset := range config.BuiltinPresets {
		printer.Printf("  %s\n", name)
		printer.Printf("    Transforms: %s\n", strings.Join(preset.Transforms, ", "))
		if preset.Quality > 0 {
			printer.Printf("    Quality: %d\n", preset.Quality)
		}
	}

	if len(cfg.Presets) > 0 {
		printer.Section("Custom Presets")
		for name, preset := range cfg.Presets {
			printer.Printf("  %s\n", name)
			printer.Printf("    Transforms: %s\n", strings.Join(preset.Transforms, ", "))
			if preset.Quality > 0 {
				printer.Printf("    Quality: %d\n", preset.Quality)
			}
			if preset.Watermark != "" {
				printer.Printf("    Watermark: %s\n", preset.Watermark)
			}
		}
	}

	return nil
}

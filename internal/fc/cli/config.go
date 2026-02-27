package cli

import (
	"fmt"
	"strings"

	"github.com/abdul-hamid-achik/file.cheap/internal/fc/config"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Manage configuration",
}

var configShowCmd = &cobra.Command{
	Use:   "show",
	Short: "Show current configuration",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		if jsonOutput {
			return printer.PrintResult(cfg)
		}

		data, err := yaml.Marshal(cfg)
		if err != nil {
			return err
		}

		path, _ := config.Path()
		printer.Section("Configuration")
		printer.KeyValue("File", path)
		printer.Println()
		printer.Printf("%s", string(data))
		return nil
	},
}

var configInitCmd = &cobra.Command{
	Use:   "init",
	Short: "Create default config file",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		path, err := config.Path()
		if err != nil {
			return err
		}

		newCfg := &config.Config{
			Quality:  config.DefaultQuality,
			LogLevel: config.DefaultLogLevel,
		}

		if err := newCfg.Save(); err != nil {
			return err
		}

		printer.Success("Config created: %s", path)
		return nil
	},
}

var configSetCmd = &cobra.Command{
	Use:   "set <key> <value>",
	Short: "Set a config value",
	Long: `Set a configuration value.

Keys: quality, output_dir, parallel, overwrite, temp_dir, log_level

Examples:
  fcheap config set quality 90
  fcheap config set output_dir /tmp/output
  fcheap config set parallel 8`,
	Args: cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		key, value := args[0], args[1]

		switch key {
		case "quality":
			var q int
			if _, err := fmt.Sscanf(value, "%d", &q); err != nil || q < 1 || q > 100 {
				return fmt.Errorf("quality must be 1-100")
			}
			cfg.Quality = q
		case "output_dir":
			cfg.OutputDir = value
		case "parallel":
			var p int
			if _, err := fmt.Sscanf(value, "%d", &p); err != nil || p < 1 {
				return fmt.Errorf("parallel must be a positive integer")
			}
			cfg.Parallel = p
		case "overwrite":
			cfg.Overwrite = value == "true" || value == "1" || value == "yes"
		case "temp_dir":
			cfg.TempDir = value
		case "log_level":
			cfg.LogLevel = value
		default:
			return fmt.Errorf("unknown config key: %s", key)
		}

		if err := cfg.Save(); err != nil {
			return err
		}

		printer.Success("Set %s = %s", key, value)
		return nil
	},
}

var configGetCmd = &cobra.Command{
	Use:   "get <key>",
	Short: "Get a config value",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		key := args[0]

		var value string
		switch key {
		case "quality":
			value = fmt.Sprintf("%d", cfg.Quality)
		case "output_dir":
			value = cfg.OutputDir
		case "parallel":
			value = fmt.Sprintf("%d", cfg.Parallel)
		case "overwrite":
			value = fmt.Sprintf("%v", cfg.Overwrite)
		case "temp_dir":
			value = cfg.TempDir
		case "log_level":
			value = cfg.LogLevel
		default:
			return fmt.Errorf("unknown config key: %s", key)
		}

		if jsonOutput {
			return printer.PrintResult(map[string]string{key: value})
		}
		printer.Printf("%s\n", value)
		return nil
	},
}

var configPathCmd = &cobra.Command{
	Use:   "path",
	Short: "Show config file path",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		path, err := config.Path()
		if err != nil {
			return err
		}
		if jsonOutput {
			return printer.PrintResult(map[string]string{"path": path})
		}
		printer.Println(path)
		return nil
	},
}

var configPresetsCmd = &cobra.Command{
	Use:   "presets",
	Short: "List available presets",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		if jsonOutput {
			allPresets := make(map[string]config.Preset)
			for name, preset := range config.BuiltinPresets {
				allPresets[name] = preset
			}
			for name, preset := range cfg.Presets {
				allPresets[name] = preset
			}
			return printer.PrintResult(allPresets)
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
	},
}

func init() {
	configCmd.AddCommand(configShowCmd)
	configCmd.AddCommand(configInitCmd)
	configCmd.AddCommand(configSetCmd)
	configCmd.AddCommand(configGetCmd)
	configCmd.AddCommand(configPathCmd)
	configCmd.AddCommand(configPresetsCmd)
}

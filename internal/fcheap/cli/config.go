package cli

import (
	"fmt"
	"strconv"

	"github.com/abdul-hamid-achik/file.cheap/internal/fcheap/config"
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
			StashDir:          cfg.StashDir,
			Compression:       config.DefaultCompression,
			CompressThreshold: config.DefaultCompressThreshold,
			LogLevel:          config.DefaultLogLevel,
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

Keys: stash_dir, compression, compress_threshold, log_level, vecgrep_path, embedder, embed_model, ollama_url

Examples:
  fcheap config set stash_dir ~/.local/share/fcheap
  fcheap config set compression zstd
  fcheap config set compress_threshold 10485760`,
	Args: cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		key, value := args[0], args[1]

		switch key {
		case "stash_dir":
			cfg.StashDir = value
		case "compression":
			cfg.Compression = value
		case "compress_threshold":
			n, err := strconv.ParseInt(value, 10, 64)
			if err != nil || n <= 0 {
				return fmt.Errorf("compress_threshold must be a positive integer")
			}
			cfg.CompressThreshold = n
		case "log_level":
			cfg.LogLevel = value
		case "vecgrep_path":
			cfg.VecgrepPath = value
		case "embedder":
			cfg.Embedder = value
		case "embed_model":
			cfg.EmbedModel = value
		case "ollama_url":
			cfg.OllamaURL = value
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
		case "stash_dir":
			value = cfg.StashDir
		case "compression":
			value = cfg.Compression
		case "compress_threshold":
			value = fmt.Sprintf("%d", cfg.CompressThreshold)
		case "log_level":
			value = cfg.LogLevel
		case "vecgrep_path":
			value = cfg.VecgrepPath
		case "embedder":
			value = cfg.Embedder
		case "embed_model":
			value = cfg.EmbedModel
		case "ollama_url":
			value = cfg.OllamaURL
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

func init() {
	configCmd.AddCommand(configShowCmd)
	configCmd.AddCommand(configInitCmd)
	configCmd.AddCommand(configSetCmd)
	configCmd.AddCommand(configGetCmd)
	configCmd.AddCommand(configPathCmd)
}

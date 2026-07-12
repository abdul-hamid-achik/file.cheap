package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/abdul-hamid-achik/file.cheap/internal/fcheap/config"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

var configInitForce bool

type configInitOutput struct {
	Action  string             `json:"action"`
	Path    string             `json:"path"`
	Status  string             `json:"status"`
	Changed bool               `json:"changed"`
	Config  configInitDocument `json:"config"`
}

type configSetOutput struct {
	Action  string             `json:"action"`
	Path    string             `json:"path"`
	Status  string             `json:"status"`
	Changed bool               `json:"changed"`
	Key     string             `json:"key"`
	Value   string             `json:"value"`
	Config  configInitDocument `json:"config"`
}

// configInitDocument intentionally omits `omitempty`: config output should show
// every supported key, including false privacy controls and empty TTL policy
// fields, instead of changing shape based on current values.
type configInitDocument struct {
	StashDir           string            `yaml:"stash_dir" json:"stash_dir"`
	Compression        string            `yaml:"compression" json:"compression"`
	CompressThreshold  int64             `yaml:"compress_threshold" json:"compress_threshold"`
	LogLevel           string            `yaml:"log_level" json:"log_level"`
	VecgrepPath        string            `yaml:"vecgrep_path" json:"vecgrep_path"`
	Embedder           string            `yaml:"embedder" json:"embedder"`
	EmbedModel         string            `yaml:"embed_model" json:"embed_model"`
	OllamaURL          string            `yaml:"ollama_url" json:"ollama_url"`
	AllowRemoteSecrets bool              `yaml:"allow_remote_secrets" json:"allow_remote_secrets"`
	DefaultTTL         string            `yaml:"default_ttl" json:"default_ttl"`
	TTLRules           map[string]string `yaml:"ttl_rules" json:"ttl_rules"`
}

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Manage configuration",
}

var configShowCmd = &cobra.Command{
	Use:   "show",
	Short: "Show current configuration",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		doc := configDocument(cfg)
		if printer.IsJSON() {
			return printer.JSON(doc)
		}

		data, err := yaml.Marshal(doc)
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
	Short: "Create a fresh default config file",
	Long: `Write a fresh config.yaml with default values.

This overwrites any existing config (resetting compression/embedder/vecgrep keys
to defaults), so it requires --force when a config file already exists.

The generated config includes allow_remote_secrets=false plus default_ttl and
ttl_rules keys (empty by default).`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		path, err := config.Path()
		if err != nil {
			return err
		}

		_, statErr := os.Stat(path)
		exists := statErr == nil
		if statErr != nil && !os.IsNotExist(statErr) {
			return fmt.Errorf("stat config: %w", statErr)
		}
		if exists && !configInitForce {
			diskCfg, err := config.LoadFromDisk()
			if err != nil {
				return err
			}
			out := configInitOutput{
				Action: "init", Path: path, Status: "exists", Changed: false, Config: configDocument(diskCfg),
			}
			if printer.IsJSON() {
				if err := printer.JSON(out); err != nil {
					return err
				}
			} else {
				printer.Warn("Config already exists: %s", path)
				printer.Warn("Use --force to overwrite (resets compression/embedder/vecgrep keys to defaults).")
			}
			return fmt.Errorf("config already exists: %s (use --force to overwrite)", path)
		}

		stashDir, err := config.DefaultStashDir()
		if err != nil {
			return err
		}
		newCfg := &config.Config{
			StashDir:          stashDir,
			Compression:       config.DefaultCompression,
			CompressThreshold: config.DefaultCompressThreshold,
			LogLevel:          config.DefaultLogLevel,
			DefaultTTL:        config.DefaultDefaultTTL,
			TTLRules:          map[string]string{},
		}

		if err := writeInitialConfig(path, newCfg); err != nil {
			return err
		}

		status := "created"
		if exists {
			status = "overwritten"
		}
		out := configInitOutput{
			Action: "init", Path: path, Status: status, Changed: true, Config: configDocument(newCfg),
		}
		if printer.IsJSON() {
			return printer.JSON(out)
		}
		printer.Success("Config created: %s", path)
		return nil
	},
}

var configSetCmd = &cobra.Command{
	Use:   "set <key> <value>",
	Short: "Set a config value",
	Long: `Set a configuration value.

Keys: stash_dir, compression, compress_threshold, log_level, vecgrep_path, embedder, embed_model, ollama_url, allow_remote_secrets, default_ttl, ttl_rules

Examples:
  fcheap config set stash_dir ~/.local/share/fcheap
  fcheap config set compression zstd
  fcheap config set compress_threshold 10485760
  fcheap config set allow_remote_secrets false
  fcheap config set default_ttl 14d
  fcheap config set ttl_rules vidtrace=30d,codemap=7d`,
	Args: cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		key, value := args[0], args[1]

		// Persist against the ON-DISK config (no env/flag overrides), so a
		// transient FCHEAP_* env var or --stash-dir is never baked into the file.
		diskCfg, err := config.LoadFromDisk()
		if err != nil {
			return err
		}

		switch key {
		case "stash_dir":
			diskCfg.StashDir = value
		case "compression":
			switch value {
			case "zstd", "gzip", "none":
			default:
				return fmt.Errorf("compression must be one of: zstd, gzip, none")
			}
			diskCfg.Compression = value
		case "compress_threshold":
			n, err := strconv.ParseInt(value, 10, 64)
			if err != nil || n <= 0 {
				return fmt.Errorf("compress_threshold must be a positive integer")
			}
			diskCfg.CompressThreshold = n
		case "log_level":
			switch value {
			case "debug", "info", "warn", "error":
			default:
				return fmt.Errorf("log_level must be one of: debug, info, warn, error")
			}
			diskCfg.LogLevel = value
		case "vecgrep_path":
			diskCfg.VecgrepPath = value
		case "embedder":
			diskCfg.Embedder = value
		case "embed_model":
			diskCfg.EmbedModel = value
		case "ollama_url":
			diskCfg.OllamaURL = value
		case "allow_remote_secrets":
			allow, err := strconv.ParseBool(value)
			if err != nil {
				return fmt.Errorf("allow_remote_secrets must be true or false")
			}
			diskCfg.AllowRemoteSecrets = allow
		case "default_ttl":
			diskCfg.DefaultTTL = value
		case "ttl_rules":
			rules, err := parseTTLRules(value)
			if err != nil {
				return err
			}
			diskCfg.TTLRules = rules
		default:
			return fmt.Errorf("unknown config key: %s", key)
		}

		path, err := config.Path()
		if err != nil {
			return err
		}
		// Keep the same explicit, complete on-disk shape produced by `config init`;
		// Config.Save uses omitempty and would otherwise make supported empty keys
		// disappear after the first `config set`.
		if err := writeInitialConfig(path, diskCfg); err != nil {
			return err
		}
		if printer.IsJSON() {
			return printer.JSON(configSetOutput{
				Action: "set", Path: path, Status: "updated", Changed: true, Key: key, Value: value,
				Config: configDocument(diskCfg),
			})
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
		case "allow_remote_secrets":
			value = strconv.FormatBool(cfg.AllowRemoteSecrets)
		case "default_ttl":
			value = cfg.DefaultTTL
		case "ttl_rules":
			value = formatTTLRules(cfg.TTLRules)
		default:
			return fmt.Errorf("unknown config key: %s", key)
		}

		if printer.IsJSON() {
			return printer.JSON(map[string]string{key: value})
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
		if printer.IsJSON() {
			return printer.JSON(map[string]string{"path": path})
		}
		printer.Println(path)
		return nil
	},
}

func init() {
	configInitCmd.Flags().BoolVar(&configInitForce, "force", false, "Overwrite an existing config file")
	configCmd.AddCommand(configShowCmd)
	configCmd.AddCommand(configInitCmd)
	configCmd.AddCommand(configSetCmd)
	configCmd.AddCommand(configGetCmd)
	configCmd.AddCommand(configPathCmd)
}

// parseTTLRules parses a comma-separated key=value string into a map.
// Example: "vidtrace=30d,codemap=7d" -> {"vidtrace":"30d", "codemap":"7d"}
func parseTTLRules(s string) (map[string]string, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, nil
	}
	rules := make(map[string]string)
	for _, pair := range strings.Split(s, ",") {
		pair = strings.TrimSpace(pair)
		if pair == "" {
			continue
		}
		parts := strings.SplitN(pair, "=", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("invalid ttl_rule %q (expected key=value, e.g. vidtrace=30d)", pair)
		}
		key := strings.TrimSpace(parts[0])
		val := strings.TrimSpace(parts[1])
		if key == "" {
			return nil, fmt.Errorf("invalid ttl_rule %q (empty tool name)", pair)
		}
		rules[key] = val
	}
	return rules, nil
}

// formatTTLRules renders a TTL rules map back to the comma-separated
// key=value form used by `config set ttl_rules`.
func formatTTLRules(rules map[string]string) string {
	if len(rules) == 0 {
		return ""
	}
	var parts []string
	for k, v := range rules {
		parts = append(parts, k+"="+v)
	}
	// Sort for stable output.
	sort.Strings(parts)
	return strings.Join(parts, ",")
}

func writeInitialConfig(path string, cfg *config.Config) error {
	doc := configDocument(cfg)
	data, err := yaml.Marshal(doc)
	if err != nil {
		return fmt.Errorf("marshal initial config: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return fmt.Errorf("create config directory: %w", err)
	}
	if err := os.WriteFile(path, data, 0600); err != nil {
		return fmt.Errorf("write config: %w", err)
	}
	if err := os.Chmod(path, 0600); err != nil {
		return fmt.Errorf("set config permissions: %w", err)
	}
	return nil
}

func configDocument(cfg *config.Config) configInitDocument {
	if cfg == nil {
		return configInitDocument{TTLRules: map[string]string{}}
	}
	doc := configInitDocument{
		StashDir:           cfg.StashDir,
		Compression:        cfg.Compression,
		CompressThreshold:  cfg.CompressThreshold,
		LogLevel:           cfg.LogLevel,
		VecgrepPath:        cfg.VecgrepPath,
		Embedder:           cfg.Embedder,
		EmbedModel:         cfg.EmbedModel,
		OllamaURL:          cfg.OllamaURL,
		AllowRemoteSecrets: cfg.AllowRemoteSecrets,
		DefaultTTL:         cfg.DefaultTTL,
		TTLRules:           cfg.TTLRules,
	}
	if doc.TTLRules == nil {
		doc.TTLRules = map[string]string{}
	}
	return doc
}

package config

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	glog "github.com/henrocdotnet/grumbler/internal/log"
	"gopkg.in/yaml.v3"
)

// Config is the top-level CLI configuration.
type Config struct {
	LLM    LLMConfig    `yaml:"llm"`
	Review ReviewConfig `yaml:"review"`
	Passes PassesConfig `yaml:"passes"`
	Filter FilterConfig `yaml:"filter"`
	Output OutputConfig `yaml:"output"`
}

type LLMConfig struct {
	Provider  string `yaml:"provider"`
	Model     string `yaml:"model"`
	APIKey    string `yaml:"apiKey"`
	MaxTokens int    `yaml:"maxTokens"`
	BaseURL   string `yaml:"baseUrl,omitempty"`
}

type ReviewConfig struct {
	BaseBranch  string   `yaml:"baseBranch"`
	Concurrency int      `yaml:"concurrency"`
	IgnorePaths []string `yaml:"ignorePaths"`
	Mode        string   `yaml:"mode"` // "diff" (default) or "file"
}

type PassesConfig struct {
	ReviewTeam bool `yaml:"reviewTeam"`
	Audit      bool `yaml:"audit"`
	CrossFile  bool `yaml:"crossFile"`
}

type FilterConfig struct {
	MinSeverity    string `yaml:"minSeverity"`
	MaxSuggestions int    `yaml:"maxSuggestions"`
}

type OutputConfig struct {
	Format  string `yaml:"format"`
	Publish bool   `yaml:"publish"`
}

// Dir is the config directory name, created by `init` and read by all subsystems.
const Dir = ".grumbler"

var envVarRe = regexp.MustCompile(`\$\{([^}]+)\}`)

// expandEnvVars replaces ${VAR_NAME} with the corresponding env value.
func expandEnvVars(s string) string {
	return envVarRe.ReplaceAllStringFunc(s, func(match string) string {
		key := envVarRe.FindStringSubmatch(match)[1]
		if val, ok := os.LookupEnv(key); ok {
			return val
		}
		return match
	})
}

// GlobalDir returns the global config directory path.
// Respects XDG_CONFIG_HOME; defaults to ~/.config/grumbler.
func GlobalDir() string {
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		return filepath.Join(xdg, "grumbler")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".config", "grumbler")
}

// Load builds config by cascading: defaults → global → project-local.
// Each layer only overwrites fields explicitly present in its YAML.
func Load(projectDir string) (*Config, error) {
	cfg := Defaults()

	globalPath := filepath.Join(GlobalDir(), "config.yaml")
	projectPath := filepath.Join(projectDir, Dir, "config.yaml")
	glog.L().Debug("config paths", "global", globalPath, "project", projectPath)

	// Layer 1: global config
	if err := mergeFromFile(cfg, globalPath); err != nil {
		return nil, fmt.Errorf("global config: %w", err)
	}

	// Layer 2: project-local config
	if err := mergeFromFile(cfg, projectPath); err != nil {
		return nil, fmt.Errorf("project config: %w", err)
	}

	glog.L().Debug("config loaded", "provider", cfg.LLM.Provider, "model", cfg.LLM.Model)
	return cfg, nil
}

// mergeFromFile reads a YAML file and unmarshals it on top of cfg.
// Missing files are silently skipped.
func mergeFromFile(cfg *Config, path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			glog.L().Debug("config file not found", "path", path)
			return nil
		}
		return err
	}
	glog.L().Debug("config file loaded", "path", path, "bytes", len(data))
	expanded := expandEnvVars(string(data))
	return yaml.Unmarshal([]byte(expanded), cfg)
}

// ProviderKind returns the category of the configured provider.
func (c *LLMConfig) ProviderKind() string {
	switch strings.ToLower(c.Provider) {
	case "claude-cli":
		return "cli"
	case "gemini-cli":
		return "cli"
	default:
		return "api"
	}
}

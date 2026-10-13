package config

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

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

	// ReasoningEffort is passed to codex-cli as model_reasoning_effort.
	// One of: minimal|low|medium|high|xhigh|max|ultra. Empty = provider default.
	ReasoningEffort string `yaml:"reasoningEffort,omitempty"`
	// Sandbox is the codex-cli sandbox policy: read-only|workspace-write|danger-full-access.
	Sandbox string `yaml:"sandbox,omitempty"`
	// CallTimeout bounds a single provider call, e.g. "5m". Empty/"0" = none.
	CallTimeout string `yaml:"callTimeout,omitempty"`
}

type ReviewConfig struct {
	BaseBranch  string   `yaml:"baseBranch"`
	Concurrency int      `yaml:"concurrency"`
	IgnorePaths []string `yaml:"ignorePaths"`
	Mode        string   `yaml:"mode"` // "diff" (default) or "file"

	// IncludeFileContent embeds each changed file's full current content in the
	// inspect prompt alongside its diff. On by default.
	//
	// MEASURED on a 143-file branch at maxBatchTokens 10000: content on
	// raises prompt tokens 2.1x (248k -> 773k) and batch count 1.8x (40 -> 71),
	// but is FASTER end to end (4m50s vs 5m42s) and recovers far more (17 of 19
	// baseline files vs 12). Per-call latency halves (22s vs 42s) because
	// reasoning, not input size, dominates codex latency: given the file, the
	// model reads instead of inferring.
	IncludeFileContent bool `yaml:"includeFileContent"`
	// MaxFileContentTokens caps per-file embedded content when
	// IncludeFileContent is on. 0 = unlimited.
	MaxFileContentTokens int `yaml:"maxFileContentTokens"`
	// MaxBatchTokens is the token budget for one inspect batch.
	MaxBatchTokens int `yaml:"maxBatchTokens"`
	// DiffContextLines is git diff --unified=N. A wider window partly replaces
	// the whole-file context dropped when IncludeFileContent is off.
	DiffContextLines int `yaml:"diffContextLines"`
	// Timeout bounds the whole review, e.g. "30m". Empty/"0" = none.
	Timeout string `yaml:"timeout,omitempty"`
}

type PassesConfig struct {
	ReviewTeam bool `yaml:"reviewTeam"`
	Audit      bool `yaml:"audit"`
	CrossFile  bool `yaml:"crossFile"`

	// MaxContextTokens caps the whole-diff prompts built by the compliance and
	// crossfile passes, which are otherwise unbounded and overflow the model's
	// context window on large branches.
	MaxContextTokens int `yaml:"maxContextTokens"`
	// OnOverflow selects behaviour when those prompts exceed the budget:
	// "truncate" (default) or "skip".
	OnOverflow string `yaml:"onOverflow,omitempty"`
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

	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	glog.L().Debug("config loaded", "provider", cfg.LLM.Provider, "model", cfg.LLM.Model)
	return cfg, nil
}

// parseDuration accepts "", "0" and any time.ParseDuration string.
// An empty or zero value means "no limit".
func parseDuration(field, v string) (time.Duration, error) {
	v = strings.TrimSpace(v)
	if v == "" || v == "0" {
		return 0, nil
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		return 0, fmt.Errorf("%s: invalid duration %q: %w", field, v, err)
	}
	if d < 0 {
		return 0, fmt.Errorf("%s: duration must not be negative, got %q", field, v)
	}
	return d, nil
}

// CallTimeoutDuration returns the per-provider-call timeout. 0 = no limit.
func (l LLMConfig) CallTimeoutDuration() time.Duration {
	d, _ := parseDuration("llm.callTimeout", l.CallTimeout)
	return d
}

// TimeoutDuration returns the whole-run timeout. 0 = no limit.
func (r ReviewConfig) TimeoutDuration() time.Duration {
	d, _ := parseDuration("review.timeout", r.Timeout)
	return d
}

var validReasoningEfforts = map[string]bool{
	"minimal": true, "low": true, "medium": true,
	"high": true, "xhigh": true, "max": true, "ultra": true,
}

var validSandboxes = map[string]bool{
	"read-only": true, "workspace-write": true, "danger-full-access": true,
}

// Validate checks the fields that would otherwise fail late, mid-run.
func (c *Config) Validate() error {
	if _, err := parseDuration("llm.callTimeout", c.LLM.CallTimeout); err != nil {
		return err
	}
	if _, err := parseDuration("review.timeout", c.Review.Timeout); err != nil {
		return err
	}
	if e := c.LLM.ReasoningEffort; e != "" && !validReasoningEfforts[e] {
		return fmt.Errorf("llm.reasoningEffort: unknown value %q (want minimal|low|medium|high|xhigh|max|ultra)", e)
	}
	if sb := c.LLM.Sandbox; sb != "" && !validSandboxes[sb] {
		return fmt.Errorf("llm.sandbox: unknown value %q (want read-only|workspace-write|danger-full-access)", sb)
	}
	switch c.Passes.OnOverflow {
	case "", "truncate", "skip":
	default:
		return fmt.Errorf("passes.onOverflow: unknown value %q (want truncate|skip)", c.Passes.OnOverflow)
	}
	if c.Review.DiffContextLines < 0 {
		return fmt.Errorf("review.diffContextLines: must not be negative, got %d", c.Review.DiffContextLines)
	}
	return nil
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
	case "codex-cli":
		return "cli"
	default:
		return "api"
	}
}

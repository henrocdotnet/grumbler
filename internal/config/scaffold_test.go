package config

import (
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func newReader(s string) *strings.Reader { return strings.NewReader(s) }

// The init scaffolds are the documentation users actually read. A typo or a key
// that no longer exists is invisible otherwise, because yaml.Unmarshal ignores
// unknown fields by default.
func TestScaffoldsParseAndValidate(t *testing.T) {
	for name, doc := range map[string]string{
		"project": DefaultConfigYAML,
		"global":  DefaultGlobalConfigYAML,
	} {
		t.Run(name, func(t *testing.T) {
			cfg := Defaults()
			if err := yaml.Unmarshal([]byte(doc), cfg); err != nil {
				t.Fatalf("scaffold does not parse: %v", err)
			}
			if err := cfg.Validate(); err != nil {
				t.Fatalf("scaffold does not validate: %v", err)
			}
		})
	}
}

func TestScaffoldsUseKnownKeys(t *testing.T) {
	for name, doc := range map[string]string{
		"project": DefaultConfigYAML,
		"global":  DefaultGlobalConfigYAML,
	} {
		t.Run(name, func(t *testing.T) {
			var cfg Config
			dec := yaml.NewDecoder(newReader(doc))
			dec.KnownFields(true)
			if err := dec.Decode(&cfg); err != nil {
				t.Fatalf("scaffold contains a key the Config struct does not define: %v", err)
			}
		})
	}
}

func TestValidateRejectsBadValues(t *testing.T) {
	cases := map[string]func(*Config){
		"bad reasoning effort": func(c *Config) { c.LLM.ReasoningEffort = "turbo" },
		"bad sandbox":          func(c *Config) { c.LLM.Sandbox = "wide-open" },
		"bad call timeout":     func(c *Config) { c.LLM.CallTimeout = "5 minutes" },
		"bad run timeout":      func(c *Config) { c.Review.Timeout = "later" },
		"bad overflow mode":    func(c *Config) { c.Passes.OnOverflow = "explode" },
		"negative context":     func(c *Config) { c.Review.DiffContextLines = -1 },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			cfg := Defaults()
			mutate(cfg)
			if err := cfg.Validate(); err == nil {
				t.Errorf("Validate() = nil, want an error")
			}
		})
	}
}

func TestDurationAccessors(t *testing.T) {
	cfg := Defaults()
	if got := cfg.LLM.CallTimeoutDuration(); got.Minutes() != 5 {
		t.Errorf("default callTimeout = %v, want 5m", got)
	}
	if got := cfg.Review.TimeoutDuration(); got.Minutes() != 30 {
		t.Errorf("default review timeout = %v, want 30m", got)
	}

	cfg.LLM.CallTimeout = ""
	cfg.Review.Timeout = "0"
	if got := cfg.LLM.CallTimeoutDuration(); got != 0 {
		t.Errorf("empty callTimeout = %v, want 0 (no limit)", got)
	}
	if got := cfg.Review.TimeoutDuration(); got != 0 {
		t.Errorf(`"0" timeout = %v, want 0 (no limit)`, got)
	}
}

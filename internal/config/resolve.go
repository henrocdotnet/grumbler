package config

// Resolve merges overrides from CLI flags into the loaded config.
// Precedence: flags > env > file > defaults.
type Overrides struct {
	Provider    string
	Model       string
	BaseBranch  string
	Format      string
	Concurrency int
	MinSeverity string
	MaxTokens   int
	Publish     *bool // nil = not set by flag
}

// Apply merges non-zero overrides into cfg.
func (o *Overrides) Apply(cfg *Config) {
	if o.Provider != "" {
		cfg.LLM.Provider = o.Provider
	}
	if o.Model != "" {
		cfg.LLM.Model = o.Model
	}
	if o.BaseBranch != "" {
		cfg.Review.BaseBranch = o.BaseBranch
	}
	if o.Format != "" {
		cfg.Output.Format = o.Format
	}
	if o.Concurrency > 0 {
		cfg.Review.Concurrency = o.Concurrency
	}
	if o.MinSeverity != "" {
		cfg.Filter.MinSeverity = o.MinSeverity
	}
	if o.MaxTokens > 0 {
		cfg.LLM.MaxTokens = o.MaxTokens
	}
	if o.Publish != nil {
		cfg.Output.Publish = *o.Publish
	}
}

package main

import (
	"errors"
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/redpanda-data/benthos/v4/internal/bloblang2/go/agentexam"
	"github.com/redpanda-data/benthos/v4/internal/bloblang2/go/agentexam/agents"
)

// Config is the top-level YAML configuration for a speccondenser run.
type Config struct {
	SpecDir     string   `yaml:"spec_dir"`
	TestsDir    string   `yaml:"tests_dir"`
	Categories  []string `yaml:"categories"`
	ArtifactDir string   `yaml:"artifact_dir"`
	Verbose     bool     `yaml:"verbose"`
	VerboseFile string   `yaml:"verbose_file"`
	KeepDir     bool     `yaml:"keep_dir"`

	Condense CondenseConfig `yaml:"condense"`
	Scoring  ScoringConfig  `yaml:"scoring"`
}

// CondenseConfig defines the outer condense phase.
type CondenseConfig struct {
	Agent   AgentConfig   `yaml:"agent"`
	Timeout time.Duration `yaml:"timeout"`

	// SpecFile is a path to a pre-condensed spec file. When set, the
	// condense agent is skipped and this file is used directly for scoring.
	SpecFile string `yaml:"spec_file"`
}

// ScoringConfig defines the inner scoring phase.
type ScoringConfig struct {
	Pools []PoolConfig `yaml:"pools"`
}

// PoolConfig defines a pool of agent runs for scoring.
type PoolConfig struct {
	Name        string        `yaml:"name"`
	Agent       AgentConfig   `yaml:"agent"`
	Runs        int           `yaml:"runs"`
	Timeout     time.Duration `yaml:"timeout"`
	Concurrency int           `yaml:"concurrency"`
}

// AgentConfig describes an agent to construct.
type AgentConfig struct {
	Type     string `yaml:"type"`
	Model    string `yaml:"model"`
	BaseURL  string `yaml:"base_url"`
	MaxTurns int    `yaml:"max_turns"`
	Command  string `yaml:"command"`
	NoThink  bool   `yaml:"no_think"`
}

func loadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading config: %w", err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parsing config: %w", err)
	}

	if err := cfg.validate(); err != nil {
		return nil, err
	}

	cfg.applyDefaults()
	return &cfg, nil
}

func (c *Config) validate() error {
	if c.SpecDir == "" {
		return errors.New("spec_dir is required")
	}
	if c.Condense.SpecFile == "" && c.Condense.Agent.Type == "" {
		return errors.New("condense.agent.type is required (or set condense.spec_file to skip condensing)")
	}
	if len(c.Scoring.Pools) == 0 {
		return errors.New("scoring.pools must have at least one pool")
	}
	for i, p := range c.Scoring.Pools {
		if p.Agent.Type == "" {
			return fmt.Errorf("scoring.pools[%d].agent.type is required", i)
		}
	}
	return nil
}

func (c *Config) applyDefaults() {
	if c.ArtifactDir == "" {
		c.ArtifactDir = "."
	}
	if c.Condense.Timeout == 0 {
		c.Condense.Timeout = 60 * time.Minute
	}
	for i := range c.Scoring.Pools {
		p := &c.Scoring.Pools[i]
		if p.Runs < 1 {
			p.Runs = 1
		}
		if p.Timeout == 0 {
			p.Timeout = 10 * time.Minute
		}
		if p.Concurrency < 1 {
			p.Concurrency = 1
		}
		if p.Name == "" {
			p.Name = p.Agent.Type
			if p.Agent.Model != "" {
				p.Name += "-" + p.Agent.Model
			}
		}
	}
}

func buildAgent(cfg AgentConfig) (agentexam.Agent, error) {
	switch cfg.Type {
	case "ollama":
		o := &agents.Ollama{
			BaseURL: cfg.BaseURL,
			Model:   cfg.Model,
			NoThink: cfg.NoThink,
		}
		if cfg.MaxTurns > 0 {
			o.MaxTurns = cfg.MaxTurns
		}
		return o, nil
	case "claude":
		c := &agents.ClaudeCode{
			Model: cfg.Model,
		}
		if cfg.MaxTurns > 0 {
			c.MaxTurns = cfg.MaxTurns
		}
		if cfg.Command != "" {
			c.Command = cfg.Command
		}
		return c, nil
	case "openai":
		o := &agents.OpenAI{
			BaseURL: cfg.BaseURL,
			Model:   cfg.Model,
		}
		if cfg.MaxTurns > 0 {
			o.MaxTurns = cfg.MaxTurns
		}
		return o, nil
	case "opencode":
		o := &agents.OpenCode{
			Model: cfg.Model,
		}
		if cfg.Command != "" {
			o.Command = cfg.Command
		}
		return o, nil
	default:
		return nil, fmt.Errorf("unknown agent type %q (use ollama, openai, claude, or opencode)", cfg.Type)
	}
}

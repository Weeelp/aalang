package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
)

var ErrConfig = errors.New("config error")

type Config struct {
	SourceFile     string `json:"source_file"`
	TargetFile     string `json:"target_file"`
	InputFile      string `json:"input_file"`
	JournalFile    string `json:"journal_file"`
	InputFirstTick int    `json:"input_first_tick"`
	InputPeriod    int    `json:"input_period"`
}

func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("%w: cannot read %s: %v", ErrConfig, path, err)
	}

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("%w: cannot parse %s: %v", ErrConfig, path, err)
	}

	if cfg.JournalFile == "" {
		cfg.JournalFile = "journal.log"
	}
	if cfg.InputFirstTick == 0 {
		cfg.InputFirstTick = 100
	}
	if cfg.InputPeriod == 0 {
		cfg.InputPeriod = 100
	}
	return &cfg, nil
}

func (c *Config) Require(fields map[string]string) error {
	for name, val := range fields {
		if val == "" {
			return fmt.Errorf("%w: field %q is required", ErrConfig, name)
		}
	}
	return nil
}

package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

var ErrConfig = errors.New("config error")

type Config struct {
	SourceFile     []string `json:"source_file"`
	InputFile      []string `json:"input_file"`
	TargetDir      string   `json:"target_dir"`
	JournalFile    string   `json:"journal_file"`
	InputFirstTick int      `json:"input_first_tick"`
	InputPeriod    int      `json:"input_period"`
	CacheOff       bool     `json:"cache_off"`
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

	if cfg.TargetDir == "" {
		cfg.TargetDir = "data"
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

func (c *Config) TargetPath(src string) string {
	base := strings.TrimSuffix(filepath.Base(src), filepath.Ext(src))
	return filepath.Join(c.TargetDir, base+".bin")
}

func (c *Config) JournalPath(src string) string {
	ext := filepath.Ext(c.JournalFile)
	base := strings.TrimSuffix(c.JournalFile, ext)
	name := strings.TrimSuffix(filepath.Base(src), filepath.Ext(src))
	return base + "_" + name + ext
}

func (c *Config) CheckDuplicates(sources []string) error {
	seen := make(map[string]string, len(sources))
	for _, src := range sources {
		dst := c.TargetPath(src)
		if prev, ok := seen[dst]; ok {
			return fmt.Errorf("%s and %s both produce %s", prev, src, dst)
		}
		seen[dst] = src
	}
	return nil
}

package main

import (
	"fmt"
	"os"

	"github.com/Weeelp/aalang/internal/config"
	"github.com/Weeelp/aalang/internal/isa"
	"github.com/Weeelp/aalang/internal/proc"
)

const maxTicks = 5_000_000

func main() {
	cfgPath := "config.json"
	if len(os.Args) > 1 {
		cfgPath = os.Args[1]
	}

	cfg, err := config.Load(cfgPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if len(cfg.InputFile) != len(cfg.SourceFile) {
		fmt.Fprintf(os.Stderr, "input_file has %d items, source_file has %d\n",
			len(cfg.InputFile), len(cfg.SourceFile))
		os.Exit(1)
	}

	failed := false
	for k, src := range cfg.SourceFile {
		fmt.Printf("> %s\n", src)
		if err := run(cfg, src, cfg.InputFile[k]); err != nil {
			fmt.Printf("ERROR: %v\n", err)
			failed = true
		}
	}
	if failed {
		os.Exit(1)
	}
}

func run(cfg *config.Config, src, inputPath string) error {
	bin, err := os.ReadFile(cfg.TargetPath(src))
	if err != nil {
		return fmt.Errorf("failed to read program: %w", err)
	}
	code, data, err := isa.Load(bin)
	if err != nil {
		return fmt.Errorf("failed to load program: %w", err)
	}

	var input []byte
	if inputPath != "" {
		if input, err = os.ReadFile(inputPath); err != nil {
			return fmt.Errorf("failed to read input: %w", err)
		}
	}

	cpu := proc.NewProcessor(code, proc.BuildSchedule(input, cfg.InputFirstTick, cfg.InputPeriod))
	if err := cpu.LoadData(data); err != nil {
		return err
	}

	journal, err := os.Create(cfg.JournalPath(src))
	if err != nil {
		return fmt.Errorf("failed to create journal: %w", err)
	}
	defer journal.Close()
	cpu.Log = journal
	cpu.Cache.Enabled = !cfg.CacheOff

	for cpu.Cycle() {
		if cpu.Tick >= maxTicks {
			fmt.Println("Tick limit reached")
			break
		}
	}

	if cpu.Fault != "" {
		fmt.Println("FAULT:", cpu.Fault)
	}
	fmt.Printf("Output: %q\n", cpu.Dev.Output)
	fmt.Printf("Ticks: %d, instructions: %d, overruns: %d\n",
		cpu.Tick, cpu.Instructions, cpu.Dev.Overruns)
	fmt.Printf("Cache: enabled=%t, hits=%d, misses=%d\n",
		cpu.Cache.Enabled, cpu.Cache.Hits, cpu.Cache.Misses)
	return nil
}

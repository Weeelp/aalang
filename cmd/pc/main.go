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
	path := "config.json"
	if len(os.Args) > 1 {
		path = os.Args[1]
	}
	cfg, err := config.Load(path)
	if err == nil && len(cfg.InputFile) != len(cfg.SourceFile) {
		err = fmt.Errorf("input_file has %d items, source_file has %d",
			len(cfg.InputFile), len(cfg.SourceFile))
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	exit := 0
	for k, src := range cfg.SourceFile {
		fmt.Printf("> %s\n", src)
		if err := run(cfg, src, cfg.InputFile[k]); err != nil {
			fmt.Printf("ERROR: %v\n", err)
			exit = 1
		}
	}
	os.Exit(exit)
}

func run(cfg *config.Config, src, inputPath string) error {
	bin, err := os.ReadFile(cfg.TargetPath(src))
	if err != nil {
		return err
	}
	code, data, err := isa.Load(bin)
	if err != nil {
		return err
	}

	var input []byte
	if inputPath != "" {
		if input, err = os.ReadFile(inputPath); err != nil {
			return err
		}
	}

	cpu := proc.NewProcessor(code, proc.BuildSchedule(input, cfg.InputFirstTick, cfg.InputPeriod))
	if err := cpu.LoadData(data); err != nil {
		return err
	}
	journal, err := os.Create(cfg.JournalPath(src))
	if err != nil {
		return err
	}
	defer journal.Close()
	cpu.Log = journal
	cpu.Cache.Enabled = !cfg.CacheOff

	for cpu.Cycle() && cpu.Tick < maxTicks {
	}

	if cpu.Fault != "" {
		fmt.Println("FAULT:", cpu.Fault)
	}
	fmt.Printf("Output: %q\n", cpu.Dev.Output)
	fmt.Printf("Ticks: %d, instructions: %d, overruns: %d, cache: enabled=%t hits=%d misses=%d\n",
		cpu.Tick, cpu.Instructions, cpu.Dev.Overruns,
		cpu.Cache.Enabled, cpu.Cache.Hits, cpu.Cache.Misses)
	return nil
}

package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/Weeelp/aalang/internal/config"
	"github.com/Weeelp/aalang/internal/isa"
	"github.com/Weeelp/aalang/internal/proc"
)

const maxTicks = 5_000_000

const (
	cReset = "\033[0m"
	cFile  = "\033[38;5;110m"
	cOut   = "\033[38;5;108m"
	cNum   = "\033[38;5;180m"
	cErr   = "\033[38;5;167m"
)

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
		fmt.Fprintf(os.Stderr, "%sERROR: %v%s\n", cErr, err, cReset)
		os.Exit(1)
	}

	fail := 0
	for k, src := range cfg.SourceFile {
		fmt.Printf("\n%s[%s]%s\n", cFile, src, cReset)
		if err := run(cfg, src, cfg.InputFile[k]); err != nil {
			fmt.Printf("  %sERROR: %v%s\n", cErr, err, cReset)
			fail++
		}
	}

	statusColor := cOut
	if fail > 0 {
		statusColor = cErr
	}
	fmt.Printf("\n%s%d / %d failed.%s\n", statusColor, len(cfg.SourceFile)-fail, len(cfg.SourceFile), cReset)
	if fail > 0 {
		os.Exit(1)
	}
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
		fmt.Printf("  %sFAULT: %s%s\n", cErr, cpu.Fault, cReset)
	}

	out := strings.TrimSuffix(string(cpu.Dev.Output), "\n")
	if out != "" {
		fmt.Printf("  Output:\n")
		for _, line := range strings.Split(out, "\n") {
			fmt.Printf("    %s%s%s\n", cOut, line, cReset)
		}
	} else {
		fmt.Printf("  Output: %s(none)%s\n", cNum, cReset)
	}

	fmt.Printf("  CPU:   ticks=%s%d%s | instr=%s%d%s | overruns=%s%d%s\n",
		cNum, cpu.Tick, cReset,
		cNum, cpu.Instructions, cReset,
		cNum, cpu.Dev.Overruns, cReset)

	fmt.Printf("  Cache: enabled=%t | hits=%s%d%s | misses=%s%d%s\n",
		cpu.Cache.Enabled,
		cNum, cpu.Cache.Hits, cReset,
		cNum, cpu.Cache.Misses, cReset)

	return nil
}

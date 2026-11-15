package main

import (
	"fmt"
	"os"

	"github.com/Weeelp/aalang/internal/config"
	"github.com/Weeelp/aalang/internal/isa"
	"github.com/Weeelp/aalang/internal/proc"
)

const maxSteps = 1_000_000

func fail(format string, a ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", a...)
	os.Exit(1)
}

func main() {
	cfgPath := "config.json"
	if len(os.Args) > 1 {
		cfgPath = os.Args[1]
	}

	cfg, err := config.Load(cfgPath)
	if err != nil {
		fail("%v", err)
	}
	if err := cfg.Require(map[string]string{
		"target_file": cfg.TargetFile,
		"input_file":  cfg.InputFile,
	}); err != nil {
		fail("%v", err)
	}

	bin, err := os.ReadFile(cfg.TargetFile)
	if err != nil {
		fail("Failed to read program: %v", err)
	}
	code, data, err := isa.Load(bin)
	if err != nil {
		fail("Failed to load program: %v", err)
	}

	input, err := os.ReadFile(cfg.InputFile)
	if err != nil {
		fail("Failed to read input: %v", err)
	}

	schedule := proc.BuildSchedule(input, cfg.InputFirstTick, cfg.InputPeriod)
	cpu := proc.NewProcessor(code, schedule)
	if err := cpu.LoadData(data); err != nil {
		fail("%v", err)
	}

	journal, err := os.Create(cfg.JournalFile)
	if err != nil {
		fail("Failed to create journal: %v", err)
	}
	defer journal.Close()
	cpu.Log = journal

	steps := 0
	for cpu.Step() {
		if steps++; steps >= maxSteps {
			fmt.Println("Instruction limit reached")
			break
		}
	}

	fmt.Printf("Output: %q\n", cpu.Dev.Output)
	fmt.Printf("Instructions: %d, ticks: %d, overruns: %d\n",
		steps, cpu.Tick, cpu.Dev.Overruns)
}

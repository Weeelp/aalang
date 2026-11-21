package main

import (
	"errors"
	"fmt"
	"os"
	"sync"
	"sync/atomic"

	"github.com/Weeelp/aalang/internal/config"
	"github.com/Weeelp/aalang/internal/translator"
)

const maxG = 5

func main() {
	cfg, err := config.Load("config.json")
	if err == nil && len(cfg.SourceFile) == 0 {
		err = errors.New("source_file is empty")
	}
	if err == nil {
		err = cfg.CheckDuplicates(cfg.SourceFile)
	}
	if err == nil {
		err = os.MkdirAll(cfg.TargetDir, 0o755)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "Error:", err)
		os.Exit(1)
	}

	var failed atomic.Bool
	var wg sync.WaitGroup
	tasks := make(chan string)

	for range min(maxG, len(cfg.SourceFile)) {
		wg.Go(func() {
			for src := range tasks {
				if err := translator.Translate(src, cfg.TargetPath(src)); err != nil {
					fmt.Printf("[%s] ERROR: %v\n", src, err)
					failed.Store(true)
				}
			}
		})
	}

	for _, src := range cfg.SourceFile {
		tasks <- src
	}
	close(tasks)
	wg.Wait()

	if failed.Load() {
		os.Exit(1)
	}
}

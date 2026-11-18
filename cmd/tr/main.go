package main

import (
	"fmt"
	"os"
	"sync"

	"github.com/Weeelp/aalang/internal/config"
	"github.com/Weeelp/aalang/internal/translator"
)

const maxG = 5

func main() {
	cfg, err := config.Load("config.json")
	if err != nil {
		fmt.Printf("Config error: %v\n", err)
		return
	}
	if len(cfg.SourceFile) == 0 {
		fmt.Println("Config error: source_file is empty")
		return
	}
	if err := translator.CheckDuplicates(cfg.TargetDir, cfg.SourceFile); err != nil {
		fmt.Printf("Config error: %v\n", err)
		return
	}
	if err := os.MkdirAll(cfg.TargetDir, 0o755); err != nil {
		fmt.Printf("Cannot create %s: %v\n", cfg.TargetDir, err)
		return
	}

	tasks := make(chan string)
	var wg sync.WaitGroup

	for range min(maxG, len(cfg.SourceFile)) {
		wg.Go(func() {
			for src := range tasks {
				if err := translator.Translate(src, translator.TargetPath(cfg.TargetDir, src)); err != nil {
					fmt.Printf("[%s] ERROR: %v\n", src, err)
				}
			}
		})
	}

	for _, src := range cfg.SourceFile {
		tasks <- src
	}
	close(tasks)
	wg.Wait()
}

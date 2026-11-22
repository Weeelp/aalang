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

const (
	cReset = "\033[0m"
	cFile  = "\033[38;5;110m"
	cOut   = "\033[38;5;108m"
	cErr   = "\033[38;5;167m"
)

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
		fmt.Fprintf(os.Stderr, "%sError: %v%s\n", cErr, err, cReset)
		os.Exit(1)
	}

	var fail atomic.Int64
	var wg sync.WaitGroup
	tasks := make(chan string)

	for range min(maxG, len(cfg.SourceFile)) {
		wg.Go(func() {
			for src := range tasks {
				if err := translator.Translate(src, cfg.TargetPath(src)); err != nil {
					fmt.Printf("%s[%s]%s %sERROR: %v%s\n", cFile, src, cReset, cErr, err, cReset)
					fail.Add(1)
				}
			}
		})
	}

	for _, src := range cfg.SourceFile {
		tasks <- src
	}
	close(tasks)
	wg.Wait()

	statusColor := cOut
	if fail.Load() > 0 {
		statusColor = cErr
	}
	fmt.Printf("%s%d / %d success.%s\n", statusColor, len(cfg.SourceFile)-int(fail.Load()), len(cfg.SourceFile), cReset)

	if fail.Load() > 0 {
		os.Exit(1)
	}
}

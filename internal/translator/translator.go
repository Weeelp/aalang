package translator

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Weeelp/aalang/internal/codegen"
	"github.com/Weeelp/aalang/internal/isa"
	"github.com/Weeelp/aalang/internal/lexer"
	"github.com/Weeelp/aalang/internal/parser"
)

func TargetPath(dir, src string) string {
	base := strings.TrimSuffix(filepath.Base(src), filepath.Ext(src))
	return filepath.Join(dir, base+".bin")
}

func CheckDuplicates(dir string, sources []string) error {
	seen := make(map[string]string, len(sources))
	for _, src := range sources {
		dst := TargetPath(dir, src)
		if prev, ok := seen[dst]; ok {
			return fmt.Errorf("%s and %s both produce %s", prev, src, dst)
		}
		seen[dst] = src
	}
	return nil
}

func Translate(src, dst string) error {
	content, err := os.ReadFile(src)
	if err != nil {
		return fmt.Errorf("reading: %w", err)
	}
	fullTree, err := parser.ParseFile(lexer.Lex(content))
	if err != nil {
		return fmt.Errorf("parsing: %w", err)
	}
	code, data, err := codegen.New().Compile(fullTree)
	if err != nil {
		return fmt.Errorf("compilation: %w", err)
	}
	bin, listing, err := isa.Assemble(code, data)
	if err != nil {
		return fmt.Errorf("encoding: %w", err)
	}
	if err := os.WriteFile(dst, bin, 0o644); err != nil {
		return fmt.Errorf("writing binary: %w", err)
	}
	if err := os.WriteFile(dst+".hex", []byte(listing), 0o644); err != nil {
		return fmt.Errorf("writing listing: %w", err)
	}
	fmt.Printf("[%s] -> %s: %d instructions, %d data bytes\n", src, dst, len(code), len(data))
	return nil
}

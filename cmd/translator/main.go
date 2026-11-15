package main

import (
	"fmt"
	"os"

	"github.com/Weeelp/aalang/internal/codegen"
	"github.com/Weeelp/aalang/internal/config"
	"github.com/Weeelp/aalang/internal/isa"
	"github.com/Weeelp/aalang/internal/lexer"
	"github.com/Weeelp/aalang/internal/parser"
)

func main() {
	cfg, err := config.Load("config.json")
	if err != nil {
		fmt.Printf("Config error: %v\n", err)
		return
	}

	content, err := os.ReadFile(cfg.SourceFile)
	if err != nil {
		fmt.Printf("Error reading file: %v\n", err)
		return
	}

	tokens := lexer.Lex(content)
	fullTree, err := parser.ParseFile(tokens)
	if err != nil {
		fmt.Printf("Parsing error: %v\n", err)
		return
	}

	fmt.Println("Parsing success;")

	compiler := codegen.New()
	code, data, err := compiler.Compile(fullTree)
	if err != nil {
		fmt.Printf("Compilation error: %v\n", err)
		return
	}

	bin, listing, err := isa.Assemble(code, data)
	if err != nil {
		fmt.Printf("Encoding error: %v\n", err)
		return
	}

	if err := os.WriteFile(cfg.TargetFile, bin, 0o644); err != nil {
		fmt.Printf("Error writing binary: %v\n", err)
		return
	}
	if err := os.WriteFile(cfg.TargetFile+".hex", []byte(listing), 0o644); err != nil {
		fmt.Printf("Error writing listing: %v\n", err)
		return
	}

	fmt.Printf("Machine code written to %s (+ .hex): %d instructions, %d data bytes\n",
		cfg.TargetFile, len(code), len(data))
}

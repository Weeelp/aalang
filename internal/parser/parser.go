package parser

import (
	"errors"
	"strconv"
	"strings"
)

type Str string

func ParseFile(tokens []string) ([]any, error) {
	var tree []any
	for len(tokens) > 0 {
		var e any
		var err error
		if e, tokens, err = parseExp(tokens); err != nil {
			return nil, err
		}
		if _, ok := e.([]any); !ok {
			return nil, errors.New("top-level expression must be enclosed in brackets")
		}
		tree = append(tree, e)
	}
	return tree, nil
}

func parseExp(tokens []string) (any, []string, error) {
	tok, tokens := tokens[0], tokens[1:]

	switch {
	case tok == "(":
		var list []any
		for len(tokens) > 0 && tokens[0] != ")" {
			var e any
			var err error
			if e, tokens, err = parseExp(tokens); err != nil {
				return nil, nil, err
			}
			list = append(list, e)
		}
		if len(tokens) == 0 {
			return nil, nil, errors.New("missing closing bracket ')'")
		}
		return list, tokens[1:], nil
	case tok == ")":
		return nil, nil, errors.New("unexpected closing bracket ')'")
	case tok[0] == '"':
		raw := strings.ReplaceAll(tok, "\r", "")
		raw = strings.ReplaceAll(raw, "\n", `\n`)
		s, err := strconv.Unquote(raw)
		return Str(s), tokens, err
	}

	if n, err := strconv.Atoi(tok); err == nil {
		return n, tokens, nil
	}
	return tok, tokens, nil
}

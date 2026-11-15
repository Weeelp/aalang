package parser

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
)

type Str string

func ParseFile(tokens []string) ([]any, error) {
	var fullTree []any

	for len(tokens) > 0 {
		expr, remTokens, err := parseExp(tokens)
		if err != nil {
			return nil, err
		}
		tokens = remTokens

		if expr != nil {
			_, isSlice := expr.([]any)
			if !isSlice {
				return nil, errors.New("Syntax error: top-level expression must be enclosed in brackets")
			}
			fullTree = append(fullTree, expr)
		}
	}
	return fullTree, nil
}

func parseExp(tokens []string) (any, []string, error) {
	if len(tokens) == 0 {
		return nil, tokens, nil
	}

	token := tokens[0]
	tokens = tokens[1:]

	if token == "(" {
		var list []any
		for len(tokens) > 0 && tokens[0] != ")" {
			var expr any
			var err error
			expr, tokens, err = parseExp(tokens)
			if err != nil {
				return nil, tokens, err
			}
			list = append(list, expr)
		}

		if len(tokens) == 0 {
			return nil, tokens, errors.New("Syntax error: missing closing bracket ')'")
		}

		tokens = tokens[1:]
		return list, tokens, nil
	}

	if token == ")" {
		return nil, tokens, errors.New("Syntax error: unexpected closing bracket ')'")
	}

	if token[0] == '"' {
		s, err := parseStr(token)
		if err != nil {
			return nil, tokens, err
		}
		return s, tokens, nil
	}

	if val, err := strconv.Atoi(token); err == nil {
		return val, tokens, nil
	}

	return token, tokens, nil
}

func parseStr(tok string) (Str, error) {
	var sb strings.Builder
	for i := 1; i < len(tok); i++ {
		c := tok[i]
		switch {
		case c == '\\' && i+1 < len(tok):
			i++
			switch tok[i] {
			case 'n':
				sb.WriteByte('\n')
			case 't':
				sb.WriteByte('\t')
			case '\\', '"':
				sb.WriteByte(tok[i])
			default:
				return "", fmt.Errorf("unknown escape \\%c in string", tok[i])
			}
		case c == '"':
			if i != len(tok)-1 {
				return "", fmt.Errorf("unexpected quote inside string")
			}
			return Str(sb.String()), nil
		default:
			sb.WriteByte(c)
		}
	}
	return "", fmt.Errorf("unterminated string literal")
}

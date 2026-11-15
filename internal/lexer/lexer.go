package lexer

func Lex(content []byte) []string {
	var tokens []string
	var cur []byte

	flush := func() {
		if len(cur) > 0 {
			tokens = append(tokens, string(cur))
			cur = cur[:0]
		}
	}

	for i := 0; i < len(content); i++ {
		ch := content[i]
		switch ch {
		case '(', ')':
			flush()
			tokens = append(tokens, string(ch))
		case ' ', '\t', '\n', '\r':
			flush()
		case '"':
			flush()
			j := i + 1
			for j < len(content) && content[j] != '"' {
				if content[j] == '\\' && j+1 < len(content) {
					j++
				}
				j++
			}
			if j >= len(content) {
				tokens = append(tokens, string(content[i:]))
				return tokens
			}
			tokens = append(tokens, string(content[i:j+1]))
			i = j
		default:
			cur = append(cur, ch)
		}
	}
	flush()
	return tokens
}

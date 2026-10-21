package main

import (
	"fmt"
	"strconv"
	"strings"
)

// Функция разбиения текста на токены (добавляем пробелы вокруг скобок)
func tokenize(code string) []string {
	code = strings.ReplaceAll(code, "(", " ( ")
	code = strings.ReplaceAll(code, ")", " ) ")
	return strings.Fields(code)
}

// Парсер, который строит древовидную структуру из токенов
func parse(tokens []string) ([]interface{}, []string) {
	var result []interface{}

	for len(tokens) > 0 {
		token := tokens[0]
		tokens = tokens[1:]

		if token == "(" {
			// Нашлась открывающая скобка -> рекурсивно парсим подсписок
			subList, remainingTokens := parse(tokens)
			result = append(result, subList)
			tokens = remainingTokens
		} else if token == ")" {
			// Нашлась закрывающая скобка -> возвращаем то, что собрали внутри скобок
			return result, tokens
		} else {
			// Это атом (число или имя переменной/операции)
			if val, err := strconv.Atoi(token); err == nil {
				result = append(result, val) // Сохраняем как число
			} else {
				result = append(result, token) // Сохраняем как строку (идентификатор)
			}
		}
	}
	return result, tokens
}

func main() {
	code := "(setq x (+ 5 10))"
	tokens := tokenize(code)

	// Запускаем парсинг
	parsedTree, _ := parse(tokens)

	// Выведет структуру в виде вложенных слайсов: [[setq x [+ 5 10]]]
	fmt.Printf("%+v\n", parsedTree)
}

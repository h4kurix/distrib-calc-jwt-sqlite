package calculator

import (
	"fmt"
	"strconv"
	"unicode"
)

func isOperator(s string) bool {
	return s == "+" || s == "-" || s == "*" || s == "/"
}

// ValidateExpression проверяет символы, скобки и операнды выражения.
func ValidateExpression(expr string) error {
	if expr == "" {
		return fmt.Errorf("expression cannot be empty")
	}

	for _, ch := range expr {
		if !unicode.IsDigit(ch) && !isOperator(string(ch)) && ch != '.' && ch != '(' && ch != ')' {
			return fmt.Errorf("invalid character %q", ch)
		}
	}

	depth := 0
	for _, ch := range expr {
		switch ch {
		case '(':
			depth++
		case ')':
			depth--
			if depth < 0 {
				return fmt.Errorf("mismatched parentheses")
			}
		}
	}
	if depth != 0 {
		return fmt.Errorf("mismatched parentheses")
	}

	// Операнд должен быть числом. Пустые части появляются только у унарных знаков.
	runes := []rune(expr)
	for i := 0; i < len(runes); i++ {
		if !unicode.IsDigit(runes[i]) && runes[i] != '.' {
			continue
		}
		start := i
		for i < len(runes) && (unicode.IsDigit(runes[i]) || runes[i] == '.') {
			i++
		}
		if _, err := strconv.ParseFloat(string(runes[start:i]), 64); err != nil {
			return fmt.Errorf("invalid operand %q", string(runes[start:i]))
		}
		i--
	}
	return nil
}

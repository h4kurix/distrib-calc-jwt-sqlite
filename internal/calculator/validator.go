package calculator

import (
	"fmt"
	"strconv"
	"strings"
	"unicode"
)

// Helper function to check if a string is an operator
func isOperator(s string) bool {
	return s == "+" || s == "-" || s == "*" || s == "/"
}

// ValidateExpression checks if the expression is valid for processing
func ValidateExpression(expr string) error {
	if len(expr) == 0 {
		return fmt.Errorf("expression cannot be empty")
	}

	// Check for invalid characters
	for i, ch := range expr {
		if !unicode.IsDigit(ch) && ch != '+' && ch != '-' && ch != '*' && ch != '/' && ch != '.' && ch != '(' && ch != ')' {
			return fmt.Errorf("invalid character at position %d: %c", i, ch)
		}
	}

	// Check for consecutive operators
	for i := 0; i < len(expr)-1; i++ {
		if isOperator(string(expr[i])) && isOperator(string(expr[i+1])) {
			return fmt.Errorf("consecutive operators at position %d", i)
		}
	}

	// Check for mismatched parentheses
	openCount := 0
	for _, ch := range expr {
		if ch == '(' {
			openCount++
		} else if ch == ')' {
			openCount--
			if openCount < 0 {
				return fmt.Errorf("mismatched parentheses")
			}
		}
	}
	if openCount != 0 {
		return fmt.Errorf("mismatched parentheses")
	}

	// Check for valid operands
	tokens := strings.FieldsFunc(expr, func(r rune) bool {
		return r == '+' || r == '-' || r == '*' || r == '/' || r == '(' || r == ')'
	})

	for _, token := range tokens {
		token = strings.TrimSpace(token)
		if token == "" {
			continue
		}

		// Try to parse as float
		_, err := strconv.ParseFloat(token, 64)
		if err != nil {
			return fmt.Errorf("invalid operand: %s", token)
		}
	}

	return nil
}

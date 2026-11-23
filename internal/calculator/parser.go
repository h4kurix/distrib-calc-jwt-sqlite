package calculator

import (
	"fmt"
	"strings"
	"unicode"
)

type token struct {
	value string
	type_ tokenType
}

type tokenType int

const (
	number tokenType = iota
	operator
	leftParen
	rightParen
)

// tokenize выполняет лексический разбор. Работает с рунами, а не с байтами.
// Унарный минус превращается в "0 - ...", что сохраняет приоритет операций.
func tokenize(expression string) ([]token, error) {
	runes := []rune(expression)
	var tokens []token
	var current strings.Builder

	flush := func() {
		if current.Len() > 0 {
			tokens = append(tokens, token{current.String(), number})
			current.Reset()
		}
	}
	unaryContext := func() bool {
		if len(tokens) == 0 {
			return true
		}
		prev := tokens[len(tokens)-1]
		return prev.type_ == operator || prev.type_ == leftParen
	}

	for i := 0; i < len(runes); i++ {
		ch := runes[i]
		switch {
		case unicode.IsDigit(ch) || ch == '.':
			current.WriteRune(ch)
		case ch == '+' || ch == '-' || ch == '*' || ch == '/':
			flush()
			if unaryContext() {
				if ch == '*' || ch == '/' {
					return nil, fmt.Errorf("operator %q has no left operand", ch)
				}
				if ch == '-' {
					tokens = append(tokens, token{"0", number})
				}
			}
			tokens = append(tokens, token{string(ch), operator})
		case ch == '(':
			flush()
			tokens = append(tokens, token{"(", leftParen})
		case ch == ')':
			flush()
			tokens = append(tokens, token{")", rightParen})
		default:
			return nil, fmt.Errorf("unexpected character %q", ch)
		}
	}
	flush()
	return tokens, nil
}

func precedence(op string) int {
	switch op {
	case "+", "-":
		return 1
	case "*", "/":
		return 2
	default:
		return 0
	}
}

func buildExpressionTree(tokens []token) (*Node, error) {
	var outputQueue []*Node
	var operatorStack []string

	makeNode := func(op string) error {
		if len(outputQueue) < 2 {
			return fmt.Errorf("invalid expression")
		}
		right := outputQueue[len(outputQueue)-1]
		left := outputQueue[len(outputQueue)-2]
		outputQueue = outputQueue[:len(outputQueue)-2]
		outputQueue = append(outputQueue, &Node{
			Value: op, Left: left, Right: right, Priority: precedence(op),
		})
		return nil
	}

	for _, t := range tokens {
		switch t.type_ {
		case number:
			outputQueue = append(outputQueue, &Node{Value: t.value})
		case operator:
			for len(operatorStack) > 0 &&
				precedence(operatorStack[len(operatorStack)-1]) >= precedence(t.value) &&
				operatorStack[len(operatorStack)-1] != "(" {
				op := operatorStack[len(operatorStack)-1]
				operatorStack = operatorStack[:len(operatorStack)-1]
				if err := makeNode(op); err != nil {
					return nil, err
				}
			}
			operatorStack = append(operatorStack, t.value)
		case leftParen:
			operatorStack = append(operatorStack, t.value)
		case rightParen:
			found := false
			for len(operatorStack) > 0 && operatorStack[len(operatorStack)-1] != "(" {
				op := operatorStack[len(operatorStack)-1]
				operatorStack = operatorStack[:len(operatorStack)-1]
				if err := makeNode(op); err != nil {
					return nil, err
				}
				found = true
			}
			if len(operatorStack) == 0 {
				return nil, fmt.Errorf("unbalanced parentheses")
			}
			operatorStack = operatorStack[:len(operatorStack)-1]
			_ = found
		}
	}

	for len(operatorStack) > 0 {
		op := operatorStack[len(operatorStack)-1]
		operatorStack = operatorStack[:len(operatorStack)-1]
		if op == "(" {
			return nil, fmt.Errorf("unbalanced parentheses")
		}
		if err := makeNode(op); err != nil {
			return nil, err
		}
	}
	if len(outputQueue) != 1 {
		return nil, fmt.Errorf("invalid expression")
	}
	return outputQueue[0], nil
}

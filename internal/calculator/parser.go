package calculator

import (
	"fmt"
	"strings"
	"unicode"
)

// Лексический анализ: токенизация
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

func tokenize(expression string) ([]token, error) {
	var tokens []token
	var current strings.Builder
	parenCount := 0

	for i, ch := range expression {
		switch {
		case unicode.IsDigit(ch) || ch == '.':
			current.WriteRune(ch)
			if i == len(expression)-1 || !(unicode.IsDigit(rune(expression[i+1])) || expression[i+1] == '.') {
				tokens = append(tokens, token{current.String(), number})
				current.Reset()
			}
		case ch == '+' || ch == '-' || ch == '*' || ch == '/':
			if current.Len() > 0 {
				tokens = append(tokens, token{current.String(), number})
				current.Reset()
			}
			tokens = append(tokens, token{string(ch), operator})
		case ch == '(':
			tokens = append(tokens, token{"(", leftParen})
			parenCount++
		case ch == ')':
			if current.Len() > 0 {
				tokens = append(tokens, token{current.String(), number})
				current.Reset()
			}
			tokens = append(tokens, token{")", rightParen})
			parenCount--
			if parenCount < 0 {
				return nil, fmt.Errorf("unbalanced parentheses")
			}
		}
	}
	if parenCount != 0 {
		return nil, fmt.Errorf("unbalanced parentheses")
	}
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
				if len(outputQueue) < 2 {
					return nil, fmt.Errorf("invalid expression")
				}
				right := outputQueue[len(outputQueue)-1]
				left := outputQueue[len(outputQueue)-2]
				outputQueue = outputQueue[:len(outputQueue)-2]
				outputQueue = append(outputQueue, &Node{
					Value:    op,
					Left:     left,
					Right:    right,
					Priority: precedence(op),
				})
			}
			operatorStack = append(operatorStack, t.value)
		case leftParen:
			operatorStack = append(operatorStack, t.value)
		case rightParen:
			for len(operatorStack) > 0 && operatorStack[len(operatorStack)-1] != "(" {
				op := operatorStack[len(operatorStack)-1]
				operatorStack = operatorStack[:len(operatorStack)-1]
				if len(outputQueue) < 2 {
					return nil, fmt.Errorf("invalid expression")
				}
				right := outputQueue[len(outputQueue)-1]
				left := outputQueue[len(outputQueue)-2]
				outputQueue = outputQueue[:len(outputQueue)-2]
				outputQueue = append(outputQueue, &Node{
					Value:    op,
					Left:     left,
					Right:    right,
					Priority: precedence(op),
				})
			}
			if len(operatorStack) == 0 {
				return nil, fmt.Errorf("unbalanced parentheses")
			}
			operatorStack = operatorStack[:len(operatorStack)-1]
		}
	}

	for len(operatorStack) > 0 {
		op := operatorStack[len(operatorStack)-1]
		operatorStack = operatorStack[:len(operatorStack)-1]
		if len(outputQueue) < 2 {
			return nil, fmt.Errorf("invalid expression")
		}
		right := outputQueue[len(outputQueue)-1]
		left := outputQueue[len(outputQueue)-2]
		outputQueue = outputQueue[:len(outputQueue)-2]
		outputQueue = append(outputQueue, &Node{
			Value:    op,
			Left:     left,
			Right:    right,
			Priority: precedence(op),
		})
	}

	if len(outputQueue) != 1 {
		return nil, fmt.Errorf("invalid expression")
	}
	return outputQueue[0], nil
}

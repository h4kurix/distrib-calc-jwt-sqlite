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

// Унарные операторы хранятся в стеке отдельными значениями. Они связывают
// только следующий операнд и имеют приоритет выше умножения.
const (
	unaryPlus  = "u+"
	unaryMinus = "u-"
)

// tokenize выполняет лексический разбор. Работает с рунами, а не с байтами.
// Унарный знак остаётся отдельным токеном, чтобы 2*-3 читалось как 2*(-3).
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
				switch ch {
				case '*', '/':
					return nil, fmt.Errorf("operator %q has no left operand", ch)
				case '-':
					tokens = append(tokens, token{unaryMinus, operator})
				case '+':
					tokens = append(tokens, token{unaryPlus, operator})
				}
				continue
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
	case unaryPlus, unaryMinus:
		return 3
	case "*", "/":
		return 2
	case "+", "-":
		return 1
	default:
		return 0
	}
}

// isRightAssoc: унарные знаки применяются справа налево, поэтому --2 = 2.
func isRightAssoc(op string) bool { return op == unaryPlus || op == unaryMinus }

// displayOp убирает служебный префикс унарного оператора для сообщений.
func displayOp(op string) string {
	if isRightAssoc(op) {
		return op[1:]
	}
	return op
}

// checkTokenSequence отбрасывает последовательности, которые разборщик
// иначе принял бы молча: "2+", "(+)", "2(3)", "(2)3".
func checkTokenSequence(tokens []token) error {
	if len(tokens) == 0 {
		return fmt.Errorf("empty expression")
	}
	for i, t := range tokens {
		switch t.type_ {
		case operator:
			if i == len(tokens)-1 || tokens[i+1].type_ == rightParen {
				return fmt.Errorf("operator %q has no right operand", displayOp(t.value))
			}
		case leftParen:
			if i > 0 {
				prev := tokens[i-1]
				if prev.type_ == number || prev.type_ == rightParen {
					return fmt.Errorf("missing operator before '('")
				}
			}
		case rightParen:
			if i > 0 && tokens[i-1].type_ == leftParen {
				return fmt.Errorf("empty parentheses")
			}
		case number:
			if i > 0 {
				prev := tokens[i-1]
				if prev.type_ == number || prev.type_ == rightParen {
					return fmt.Errorf("missing operator between numbers")
				}
			}
		}
	}
	return nil
}

func buildExpressionTree(tokens []token) (*Node, error) {
	if err := checkTokenSequence(tokens); err != nil {
		return nil, err
	}
	var outputQueue []*Node
	var operatorStack []string

	makeNode := func(op string) error {
		if isRightAssoc(op) {
			if len(outputQueue) < 1 {
				return fmt.Errorf("operator %q has no operand", op[1:])
			}
			operand := outputQueue[len(outputQueue)-1]
			outputQueue = outputQueue[:len(outputQueue)-1]
			// Унарный знак превращается в бинарный: 0 - x или 0 + x.
			outputQueue = append(outputQueue, &Node{
				Value: op[1:],
				Left:  &Node{Value: "0"},
				Right: operand,
			})
			return nil
		}
		if len(outputQueue) < 2 {
			return fmt.Errorf("invalid expression")
		}
		right := outputQueue[len(outputQueue)-1]
		left := outputQueue[len(outputQueue)-2]
		outputQueue = outputQueue[:len(outputQueue)-2]
		outputQueue = append(outputQueue, &Node{
			Value: op, Left: left, Right: right,
		})
		return nil
	}

	for _, t := range tokens {
		switch t.type_ {
		case number:
			outputQueue = append(outputQueue, &Node{Value: t.value})
		case operator:
			for len(operatorStack) > 0 {
				top := operatorStack[len(operatorStack)-1]
				if top == "(" || precedence(top) < precedence(t.value) {
					break
				}
				if precedence(top) == precedence(t.value) && isRightAssoc(t.value) {
					break
				}
				operatorStack = operatorStack[:len(operatorStack)-1]
				if err := makeNode(top); err != nil {
					return nil, err
				}
			}
			operatorStack = append(operatorStack, t.value)
		case leftParen:
			operatorStack = append(operatorStack, t.value)
		case rightParen:
			for len(operatorStack) > 0 && operatorStack[len(operatorStack)-1] != "(" {
				op := operatorStack[len(operatorStack)-1]
				operatorStack = operatorStack[:len(operatorStack)-1]
				if err := makeNode(op); err != nil {
					return nil, err
				}
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

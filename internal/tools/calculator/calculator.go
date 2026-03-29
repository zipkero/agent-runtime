package calculator

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"unicode"

	"agentflow/internal/state"
	"agentflow/internal/tools"
)

// Calculator 는 수식 문자열을 받아 계산 결과를 반환하는 Tool 구현체다.
type Calculator struct{}

func New() *Calculator {
	return &Calculator{}
}

func (c *Calculator) Name() string {
	return "calculator"
}

func (c *Calculator) Description() string {
	return "수식 문자열을 계산해 결과를 반환한다. 사칙연산(+, -, *, /)과 괄호를 지원한다."
}

func (c *Calculator) InputSchema() tools.Schema {
	return tools.Schema{
		Fields: []tools.FieldSchema{
			{
				Name:        "expression",
				Type:        tools.FieldTypeString,
				Description: "계산할 수식 문자열 (예: '3 + 4 * (2 - 1)')",
				Required:    true,
			},
		},
	}
}

func (c *Calculator) Execute(_ context.Context, input map[string]any) (state.ToolResult, error) {
	raw, ok := input["expression"]
	if !ok {
		return state.ToolResult{ToolName: c.Name(), IsError: true, ErrMsg: "expression 필드가 없습니다"}, nil
	}
	expr, ok := raw.(string)
	if !ok {
		return state.ToolResult{ToolName: c.Name(), IsError: true, ErrMsg: "expression 은 string 이어야 합니다"}, nil
	}

	result, err := evaluate(strings.TrimSpace(expr))
	if err != nil {
		return state.ToolResult{ToolName: c.Name(), IsError: true, ErrMsg: err.Error()}, nil
	}

	return state.ToolResult{
		ToolName: c.Name(),
		Output:   strconv.FormatFloat(result, 'f', -1, 64),
	}, nil
}

// --- 재귀하강 파서 ---

type parser struct {
	input []rune
	pos   int
}

func evaluate(expr string) (float64, error) {
	p := &parser{input: []rune(expr)}
	result, err := p.parseExpr()
	if err != nil {
		return 0, err
	}
	p.skipSpace()
	if p.pos != len(p.input) {
		return 0, fmt.Errorf("예상치 못한 문자: %q", string(p.input[p.pos:]))
	}
	return result, nil
}

// expr → term (('+' | '-') term)*
func (p *parser) parseExpr() (float64, error) {
	left, err := p.parseTerm()
	if err != nil {
		return 0, err
	}
	for {
		p.skipSpace()
		if p.pos >= len(p.input) {
			break
		}
		op := p.input[p.pos]
		if op != '+' && op != '-' {
			break
		}
		p.pos++
		right, err := p.parseTerm()
		if err != nil {
			return 0, err
		}
		if op == '+' {
			left += right
		} else {
			left -= right
		}
	}
	return left, nil
}

// term → factor (('*' | '/') factor)*
func (p *parser) parseTerm() (float64, error) {
	left, err := p.parseFactor()
	if err != nil {
		return 0, err
	}
	for {
		p.skipSpace()
		if p.pos >= len(p.input) {
			break
		}
		op := p.input[p.pos]
		if op != '*' && op != '/' {
			break
		}
		p.pos++
		right, err := p.parseFactor()
		if err != nil {
			return 0, err
		}
		if op == '*' {
			left *= right
		} else {
			if right == 0 {
				return 0, fmt.Errorf("0으로 나눌 수 없습니다")
			}
			left /= right
		}
	}
	return left, nil
}

// factor → ['-'] number | '(' expr ')'
func (p *parser) parseFactor() (float64, error) {
	p.skipSpace()
	if p.pos >= len(p.input) {
		return 0, fmt.Errorf("예상치 못한 수식 끝")
	}

	if p.input[p.pos] == '(' {
		p.pos++
		val, err := p.parseExpr()
		if err != nil {
			return 0, err
		}
		p.skipSpace()
		if p.pos >= len(p.input) || p.input[p.pos] != ')' {
			return 0, fmt.Errorf("')' 가 없습니다")
		}
		p.pos++
		return val, nil
	}

	neg := false
	if p.input[p.pos] == '-' {
		neg = true
		p.pos++
		p.skipSpace()
	}

	val, err := p.parseNumber()
	if err != nil {
		return 0, err
	}
	if neg {
		val = -val
	}
	return val, nil
}

func (p *parser) parseNumber() (float64, error) {
	start := p.pos
	for p.pos < len(p.input) && (unicode.IsDigit(p.input[p.pos]) || p.input[p.pos] == '.') {
		p.pos++
	}
	if start == p.pos {
		return 0, fmt.Errorf("숫자를 기대했지만 %q 를 만났습니다", string(p.input[p.pos:]))
	}
	return strconv.ParseFloat(string(p.input[start:p.pos]), 64)
}

func (p *parser) skipSpace() {
	for p.pos < len(p.input) && unicode.IsSpace(p.input[p.pos]) {
		p.pos++
	}
}

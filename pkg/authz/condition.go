package authz

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// ABAC condition expression language (design spec §6)
// ---------------------------------------------------
//
// A tuple's condition_expr is evaluated against the Check request's context
// map (string keys and values). Grammar (hand-rolled lexer + recursive
// descent parser, no dependencies):
//
//	expr       = orExpr
//	orExpr     = andExpr { "||" andExpr }
//	andExpr    = term { "&&" term }
//	term       = "(" expr ")" | comparison
//	comparison = operand relop operand
//	relop      = "==" | "!=" | "<" | "<=" | ">" | ">="
//	operand    = STRING | NUMBER | "now" "(" ")" | IDENT
//
//	STRING = double-quoted, with \" and \\ escapes
//	NUMBER = [-]digits[.digits]
//	IDENT  = [A-Za-z_][A-Za-z0-9_]*  — a context key
//
// Semantics:
//
//   - IDENT reads the context map; a missing key is an evaluation error.
//   - now() yields the current UTC time formatted as RFC3339
//     ("2026-07-18T21:04:05Z"), so it compares correctly as a string
//     against RFC3339 UTC literals: now() < "2026-12-31T00:00:00Z".
//   - A comparison is numeric when BOTH operands parse as numbers
//     (context values included: ctx["n"]="5" with `n < 10` is numeric);
//     otherwise it is a lexicographic string comparison.
//   - Fail closed: any lex/parse error, trailing input, or missing context
//     key makes the whole condition false. There are no side effects.
//
// Examples: `env == "staging"`, `attempts < 3 && env != "prod"`,
// `(tier == "gold" || tier == "silver") && now() < "2027-01-01T00:00:00Z"`.

// EvalCondition evaluates a condition expression against a context map.
// It fails closed: any error yields false.
func EvalCondition(expr string, ctx map[string]string) bool {
	return evalConditionAt(expr, ctx, time.Now)
}

// evalConditionAt is EvalCondition with an injectable clock (for tests).
func evalConditionAt(expr string, ctx map[string]string, now func() time.Time) bool {
	toks, err := lexCondition(expr)
	if err != nil {
		return false
	}
	p := &condParser{toks: toks, ctx: ctx, now: now}
	result, err := p.parseOr()
	if err != nil || p.peek().kind != tokEOF {
		return false
	}
	return result
}

// ---------------------------------------------------------------- lexer

type tokenKind int

const (
	tokEOF tokenKind = iota
	tokIdent
	tokString
	tokNumber
	tokOp     // == != < <= > >=
	tokAnd    // &&
	tokOr     // ||
	tokLParen // (
	tokRParen // )
)

type token struct {
	kind tokenKind
	text string
}

func lexCondition(expr string) ([]token, error) {
	var toks []token
	i := 0
	for i < len(expr) {
		c := expr[i]
		switch {
		case c == ' ' || c == '\t' || c == '\n' || c == '\r':
			i++
		case c == '(':
			toks = append(toks, token{tokLParen, "("})
			i++
		case c == ')':
			toks = append(toks, token{tokRParen, ")"})
			i++
		case c == '&':
			if i+1 >= len(expr) || expr[i+1] != '&' {
				return nil, fmt.Errorf("expected '&&' at offset %d", i)
			}
			toks = append(toks, token{tokAnd, "&&"})
			i += 2
		case c == '|':
			if i+1 >= len(expr) || expr[i+1] != '|' {
				return nil, fmt.Errorf("expected '||' at offset %d", i)
			}
			toks = append(toks, token{tokOr, "||"})
			i += 2
		case c == '=' || c == '!':
			if i+1 >= len(expr) || expr[i+1] != '=' {
				return nil, fmt.Errorf("expected '==' or '!=' at offset %d", i)
			}
			toks = append(toks, token{tokOp, string(c) + "="})
			i += 2
		case c == '<' || c == '>':
			op := string(c)
			i++
			if i < len(expr) && expr[i] == '=' {
				op += "="
				i++
			}
			toks = append(toks, token{tokOp, op})
		case c == '"':
			var sb strings.Builder
			i++
			closed := false
			for i < len(expr) {
				if expr[i] == '\\' && i+1 < len(expr) && (expr[i+1] == '"' || expr[i+1] == '\\') {
					sb.WriteByte(expr[i+1])
					i += 2
					continue
				}
				if expr[i] == '"' {
					closed = true
					i++
					break
				}
				sb.WriteByte(expr[i])
				i++
			}
			if !closed {
				return nil, fmt.Errorf("unterminated string literal")
			}
			toks = append(toks, token{tokString, sb.String()})
		case isDigit(c) || (c == '-' && i+1 < len(expr) && isDigit(expr[i+1])):
			start := i
			i++ // first digit or '-'
			seenDot := false
			for i < len(expr) && (isDigit(expr[i]) || (expr[i] == '.' && !seenDot)) {
				if expr[i] == '.' {
					seenDot = true
				}
				i++
			}
			toks = append(toks, token{tokNumber, expr[start:i]})
		case isIdentStart(c):
			start := i
			for i < len(expr) && isIdentPart(expr[i]) {
				i++
			}
			toks = append(toks, token{tokIdent, expr[start:i]})
		default:
			return nil, fmt.Errorf("unexpected character %q at offset %d", c, i)
		}
	}
	return append(toks, token{tokEOF, ""}), nil
}

func isDigit(c byte) bool      { return c >= '0' && c <= '9' }
func isIdentStart(c byte) bool { return c == '_' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') }
func isIdentPart(c byte) bool  { return isIdentStart(c) || isDigit(c) }

// ---------------------------------------------------------------- parser

// condParser is a recursive-descent parser that evaluates as it parses
// (the expressions are tiny and side-effect free, so no AST is needed).
type condParser struct {
	toks []token
	pos  int
	ctx  map[string]string
	now  func() time.Time
}

func (p *condParser) peek() token { return p.toks[p.pos] }

func (p *condParser) next() token {
	t := p.toks[p.pos]
	if t.kind != tokEOF {
		p.pos++
	}
	return t
}

func (p *condParser) parseOr() (bool, error) {
	result, err := p.parseAnd()
	if err != nil {
		return false, err
	}
	for p.peek().kind == tokOr {
		p.next()
		rhs, err := p.parseAnd()
		if err != nil {
			return false, err
		}
		result = result || rhs
	}
	return result, nil
}

func (p *condParser) parseAnd() (bool, error) {
	result, err := p.parseTerm()
	if err != nil {
		return false, err
	}
	for p.peek().kind == tokAnd {
		p.next()
		rhs, err := p.parseTerm()
		if err != nil {
			return false, err
		}
		result = result && rhs
	}
	return result, nil
}

func (p *condParser) parseTerm() (bool, error) {
	if p.peek().kind == tokLParen {
		p.next()
		result, err := p.parseOr()
		if err != nil {
			return false, err
		}
		if p.next().kind != tokRParen {
			return false, fmt.Errorf("expected ')'")
		}
		return result, nil
	}
	return p.parseComparison()
}

func (p *condParser) parseComparison() (bool, error) {
	lhs, err := p.parseOperand()
	if err != nil {
		return false, err
	}
	op := p.next()
	if op.kind != tokOp {
		return false, fmt.Errorf("expected a comparison operator, got %q", op.text)
	}
	rhs, err := p.parseOperand()
	if err != nil {
		return false, err
	}
	return compareValues(lhs, rhs, op.text)
}

// parseOperand yields the operand's string value. STRING and NUMBER are
// literals; IDENT is a context lookup (missing key = error, fail closed);
// now() is the current UTC time in RFC3339.
func (p *condParser) parseOperand() (string, error) {
	t := p.next()
	switch t.kind {
	case tokString, tokNumber:
		return t.text, nil
	case tokIdent:
		if t.text == "now" && p.peek().kind == tokLParen {
			p.next()
			if p.next().kind != tokRParen {
				return "", fmt.Errorf("expected ')' after 'now('")
			}
			return p.now().UTC().Format(time.RFC3339), nil
		}
		v, ok := p.ctx[t.text]
		if !ok {
			return "", fmt.Errorf("context key %q is not set", t.text)
		}
		return v, nil
	default:
		return "", fmt.Errorf("expected a string, number, context key, or now(), got %q", t.text)
	}
}

// compareValues compares numerically when both sides parse as numbers,
// lexicographically otherwise (RFC3339 UTC timestamps compare correctly
// as strings).
func compareValues(lhs, rhs, op string) (bool, error) {
	var cmp int
	ln, lerr := strconv.ParseFloat(lhs, 64)
	rn, rerr := strconv.ParseFloat(rhs, 64)
	if lerr == nil && rerr == nil {
		switch {
		case ln < rn:
			cmp = -1
		case ln > rn:
			cmp = 1
		}
	} else {
		cmp = strings.Compare(lhs, rhs)
	}
	switch op {
	case "==":
		return cmp == 0, nil
	case "!=":
		return cmp != 0, nil
	case "<":
		return cmp < 0, nil
	case "<=":
		return cmp <= 0, nil
	case ">":
		return cmp > 0, nil
	case ">=":
		return cmp >= 0, nil
	default:
		return false, fmt.Errorf("unknown operator %q", op)
	}
}

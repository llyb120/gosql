package gosql

import (
	"fmt"
	"strings"
	"unicode"
)

// ==================== 表达式编译器 ====================
// 将模板表达式编译为原生 Go 代码，不依赖任何运行时解释器。
//
// 支持的表达式:
//   a > 0           → gosql.GT(ctx.MustGet("a"), 0)
//   a < 0 && b > 1  → gosql.LT(ctx.MustGet("a"), 0) && gosql.GT(ctx.MustGet("b"), 1)
//   GetName()       → ctx.Call("GetName")
//   trim("and")     → ctx.Call("trim", "and")
//   len(ids) > 0    → gosql.GT(gosql.CallLen(ctx.MustGet("ids")), 0)
//   !flag           → !gosql.ToBool(ctx.MustGet("flag"))

// ==================== Token 定义 ====================

type exprTokType int

const (
	etEOF    exprTokType = iota
	etNumber             // 数字: 0, 1, 3.14
	etString             // 字符串: "hello"
	etIdent              // 标识符: a, name, GetName
	etLParen             // (
	etRParen             // )
	etComma              // ,
	etDot                // .
	etGT                 // >
	etGE                 // >=
	etLT                 // <
	etLE                 // <=
	etEQ                 // ==
	etNE                 // !=
	etAssign             // =
	etAnd                // &&
	etOr                 // ||
	etNot                // !
	etPlus               // +
	etMinus              // -
	etMod                // %
)

type exprToken struct {
	typ   exprTokType
	value string
}

// ==================== Tokenizer ====================

func tokenizeExpr(input string) ([]exprToken, error) {
	var tokens []exprToken
	i := 0
	for i < len(input) {
		ch := input[i]

		// 跳过空白
		if ch == ' ' || ch == '\t' || ch == '\n' || ch == '\r' {
			i++
			continue
		}

		// 数字
		if ch >= '0' && ch <= '9' {
			start := i
			for i < len(input) && (input[i] >= '0' && input[i] <= '9' || input[i] == '.') {
				i++
			}
			tokens = append(tokens, exprToken{typ: etNumber, value: input[start:i]})
			continue
		}

		// 字符串（双引号）
		if ch == '"' {
			start := i
			i++ // skip opening "
			for i < len(input) && input[i] != '"' {
				if input[i] == '\\' {
					i++ // skip escape
				}
				i++
			}
			if i < len(input) {
				i++ // skip closing "
			}
			tokens = append(tokens, exprToken{typ: etString, value: input[start:i]})
			continue
		}

		// 字符串（反引号）
		if ch == '`' {
			start := i
			i++ // skip opening `
			for i < len(input) && input[i] != '`' {
				i++
			}
			if i < len(input) {
				i++ // skip closing `
			}
			tokens = append(tokens, exprToken{typ: etString, value: input[start:i]})
			continue
		}

		// 标识符
		if isIdentStart(ch) {
			start := i
			for i < len(input) && isIdentPart(input[i]) {
				i++
			}
			tokens = append(tokens, exprToken{typ: etIdent, value: input[start:i]})
			continue
		}

		// 运算符
		switch ch {
		case '(':
			tokens = append(tokens, exprToken{typ: etLParen, value: "("})
			i++
		case ')':
			tokens = append(tokens, exprToken{typ: etRParen, value: ")"})
			i++
		case ',':
			tokens = append(tokens, exprToken{typ: etComma, value: ","})
			i++
		case '.':
			tokens = append(tokens, exprToken{typ: etDot, value: "."})
			i++
		case '+':
			tokens = append(tokens, exprToken{typ: etPlus, value: "+"})
			i++
		case '-':
			tokens = append(tokens, exprToken{typ: etMinus, value: "-"})
			i++
		case '%':
			tokens = append(tokens, exprToken{typ: etMod, value: "%"})
			i++
		case '>':
			if i+1 < len(input) && input[i+1] == '=' {
				tokens = append(tokens, exprToken{typ: etGE, value: ">="})
				i += 2
			} else {
				tokens = append(tokens, exprToken{typ: etGT, value: ">"})
				i++
			}
		case '<':
			if i+1 < len(input) && input[i+1] == '=' {
				tokens = append(tokens, exprToken{typ: etLE, value: "<="})
				i += 2
			} else {
				tokens = append(tokens, exprToken{typ: etLT, value: "<"})
				i++
			}
		case '=':
			if i+1 < len(input) && input[i+1] == '=' {
				tokens = append(tokens, exprToken{typ: etEQ, value: "=="})
				i += 2
			} else {
				tokens = append(tokens, exprToken{typ: etAssign, value: "="})
				i++
			}
		case '!':
			if i+1 < len(input) && input[i+1] == '=' {
				tokens = append(tokens, exprToken{typ: etNE, value: "!="})
				i += 2
			} else {
				tokens = append(tokens, exprToken{typ: etNot, value: "!"})
				i++
			}
		case '&':
			if i+1 < len(input) && input[i+1] == '&' {
				tokens = append(tokens, exprToken{typ: etAnd, value: "&&"})
				i += 2
			} else {
				return nil, fmt.Errorf("unexpected '&' at position %d", i)
			}
		case '|':
			if i+1 < len(input) && input[i+1] == '|' {
				tokens = append(tokens, exprToken{typ: etOr, value: "||"})
				i += 2
			} else {
				return nil, fmt.Errorf("unexpected '|' at position %d", i)
			}
		default:
			return nil, fmt.Errorf("unexpected character '%c' at position %d", ch, i)
		}
	}

	tokens = append(tokens, exprToken{typ: etEOF, value: ""})
	return tokens, nil
}

// ==================== 递归下降解析器 ====================
//
// 语法:
//   expr       → logic_or
//   logic_or   → logic_and ("||" logic_and)*
//   logic_and  → comparison ("&&" comparison)*
//   comparison → addition ((">" | ">=" | "<" | "<=" | "==" | "!=") addition)?
//   addition   → unary (("+" | "-") unary)*
//   unary      → "!" unary | "-" unary | primary
//   primary    → NUMBER | STRING | "true" | "false" | "nil"
//              | "len" "(" expr ")"
//              | IDENT "(" args ")"   // 函数调用
//              | IDENT "." IDENT "(" args ")"  // 方法调用
//              | IDENT                // 变量
//              | "(" expr ")"
//   args       → (expr ("," expr)*)?

type exprCompiler struct {
	tokens []exprToken
	pos    int
	pkg    string // "gosql." 或 "" (gosql包内部时)
}

func (c *exprCompiler) current() exprToken {
	if c.pos >= len(c.tokens) {
		return exprToken{typ: etEOF}
	}
	return c.tokens[c.pos]
}

func (c *exprCompiler) advance() exprToken {
	tok := c.current()
	if c.pos < len(c.tokens) {
		c.pos++
	}
	return tok
}

func (c *exprCompiler) peek() exprToken {
	return c.current()
}

func (c *exprCompiler) expect(typ exprTokType) (exprToken, error) {
	tok := c.current()
	if tok.typ != typ {
		return tok, fmt.Errorf("expected token type %d, got %d (%q)", typ, tok.typ, tok.value)
	}
	c.pos++
	return tok, nil
}

// parseExpr → logic_or
func (c *exprCompiler) parseExpr() (code string, isBool bool, err error) {
	return c.parseLogicOr()
}

// logic_or → logic_and ("||" logic_and)*
func (c *exprCompiler) parseLogicOr() (string, bool, error) {
	left, leftBool, err := c.parseLogicAnd()
	if err != nil {
		return "", false, err
	}

	for c.peek().typ == etOr {
		c.advance()
		right, rightBool, err := c.parseLogicAnd()
		if err != nil {
			return "", false, err
		}
		leftCode := c.ensureBool(left, leftBool)
		rightCode := c.ensureBool(right, rightBool)
		left = leftCode + " || " + rightCode
		leftBool = true
	}

	return left, leftBool, nil
}

// logic_and → comparison ("&&" comparison)*
func (c *exprCompiler) parseLogicAnd() (string, bool, error) {
	left, leftBool, err := c.parseComparison()
	if err != nil {
		return "", false, err
	}

	for c.peek().typ == etAnd {
		c.advance()
		right, rightBool, err := c.parseComparison()
		if err != nil {
			return "", false, err
		}
		leftCode := c.ensureBool(left, leftBool)
		rightCode := c.ensureBool(right, rightBool)
		left = leftCode + " && " + rightCode
		leftBool = true
	}

	return left, leftBool, nil
}

// comparison → addition (compOp addition)?
func (c *exprCompiler) parseComparison() (string, bool, error) {
	left, leftBool, err := c.parseAddition()
	if err != nil {
		return "", false, err
	}

	tok := c.peek()
	switch tok.typ {
	case etGT, etGE, etLT, etLE, etEQ, etNE:
		c.advance()
		right, _, err := c.parseAddition()
		if err != nil {
			return "", false, err
		}
		var fn string
		switch tok.typ {
		case etGT:
			fn = "GT"
		case etGE:
			fn = "GE"
		case etLT:
			fn = "LT"
		case etLE:
			fn = "LE"
		case etEQ:
			fn = "EQ"
		case etNE:
			fn = "NE"
		}
		return fmt.Sprintf("%s%s(%s, %s)", c.pkg, fn, left, right), true, nil
	}

	return left, leftBool, nil
}

// addition → unary (("+" | "-") unary)*
func (c *exprCompiler) parseAddition() (string, bool, error) {
	left, leftBool, err := c.parseUnary()
	if err != nil {
		return "", false, err
	}

	for c.peek().typ == etPlus || c.peek().typ == etMinus || c.peek().typ == etMod {
		op := c.advance()
		right, _, err := c.parseUnary()
		if err != nil {
			return "", false, err
		}
		var fn string
		switch op.typ {
		case etPlus:
			fn = "Add"
		case etMinus:
			fn = "Sub"
		case etMod:
			fn = "Mod"
		}
		left = fmt.Sprintf("%s%s(%s, %s)", c.pkg, fn, left, right)
		leftBool = false // 算术运算结果不是 bool
	}

	return left, leftBool, nil
}

// unary → "!" unary | "-" unary | primary
func (c *exprCompiler) parseUnary() (string, bool, error) {
	if c.peek().typ == etNot {
		c.advance()
		inner, innerBool, err := c.parseUnary()
		if err != nil {
			return "", false, err
		}
		innerCode := c.ensureBool(inner, innerBool)
		return "!" + innerCode, true, nil
	}

	if c.peek().typ == etMinus {
		c.advance()
		// 负数字面量直接合并
		if c.peek().typ == etNumber {
			tok := c.advance()
			return "-" + tok.value, false, nil
		}
		inner, _, err := c.parseUnary()
		if err != nil {
			return "", false, err
		}
		return fmt.Sprintf("%sNegate(%s)", c.pkg, inner), false, nil
	}

	return c.parsePrimary()
}

// primary → NUMBER | STRING | "true" | "false" | "nil"
//
//	| "len" "(" expr ")"
//	| IDENT "(" args ")"
//	| IDENT "." IDENT "(" args ")"
//	| IDENT
//	| "(" expr ")"
func (c *exprCompiler) parsePrimary() (string, bool, error) {
	tok := c.peek()

	switch tok.typ {
	case etNumber:
		c.advance()
		return tok.value, false, nil

	case etString:
		c.advance()
		return tok.value, false, nil

	case etIdent:
		c.advance()

		// 关键字: true, false, nil
		switch tok.value {
		case "true":
			return "true", true, nil
		case "false":
			return "false", true, nil
		case "nil":
			return "nil", false, nil
		}

		// 内建函数: len(expr)
		if tok.value == "len" && c.peek().typ == etLParen {
			c.advance() // skip (
			arg, _, err := c.parseExpr()
			if err != nil {
				return "", false, err
			}
			if _, err := c.expect(etRParen); err != nil {
				return "", false, fmt.Errorf("len() missing closing paren")
			}
			return fmt.Sprintf("%sCallLen(%s)", c.pkg, arg), false, nil
		}

		// 函数调用: name(args...)
		if c.peek().typ == etLParen {
			return c.parseFuncCall(tok.value)
		}

		// 方法调用: obj.method(args...)
		if c.peek().typ == etDot {
			c.advance() // skip .
			method, err := c.expect(etIdent)
			if err != nil {
				return "", false, fmt.Errorf("expected method name after '.'")
			}
			if c.peek().typ == etLParen {
				return c.parseMethodCall(tok.value, method.value)
			}
			// 属性访问: obj.field -> 暂不支持，使用 ctx.MustGet
			return fmt.Sprintf("ctx.MustGet(\"%s.%s\")", tok.value, method.value), false, nil
		}

		// 普通变量
		return fmt.Sprintf("ctx.MustGet(\"%s\")", tok.value), false, nil

	case etLParen:
		c.advance() // skip (
		inner, innerBool, err := c.parseExpr()
		if err != nil {
			return "", false, err
		}
		if _, err := c.expect(etRParen); err != nil {
			return "", false, fmt.Errorf("missing closing paren")
		}
		return "(" + inner + ")", innerBool, nil
	}

	return "", false, fmt.Errorf("unexpected token: %q (type %d)", tok.value, tok.typ)
}

// parseFuncCall 编译函数调用: name(args...)
// 生成: ctx.Call("name", arg1, arg2, ...)
func (c *exprCompiler) parseFuncCall(funcName string) (string, bool, error) {
	c.advance() // skip (
	args, err := c.parseArgs()
	if err != nil {
		return "", false, err
	}
	if _, err := c.expect(etRParen); err != nil {
		return "", false, fmt.Errorf("function %s() missing closing paren", funcName)
	}

	parts := []string{fmt.Sprintf("%q", funcName)}
	for _, arg := range args {
		parts = append(parts, arg)
	}
	return fmt.Sprintf("ctx.Call(%s)", strings.Join(parts, ", ")), false, nil
}

// parseMethodCall 编译方法调用: obj.method(args...)
// 生成: ctx.CallMethod("obj", "method", arg1, arg2, ...)
func (c *exprCompiler) parseMethodCall(objName, methodName string) (string, bool, error) {
	c.advance() // skip (
	args, err := c.parseArgs()
	if err != nil {
		return "", false, err
	}
	if _, err := c.expect(etRParen); err != nil {
		return "", false, fmt.Errorf("method %s.%s() missing closing paren", objName, methodName)
	}

	parts := []string{fmt.Sprintf("%q", objName), fmt.Sprintf("%q", methodName)}
	for _, arg := range args {
		parts = append(parts, arg)
	}
	return fmt.Sprintf("ctx.CallMethod(%s)", strings.Join(parts, ", ")), false, nil
}

// parseArgs 解析函数参数列表
func (c *exprCompiler) parseArgs() ([]string, error) {
	var args []string
	if c.peek().typ == etRParen {
		return args, nil // 空参数
	}

	arg, _, err := c.parseExpr()
	if err != nil {
		return nil, err
	}
	args = append(args, arg)

	for c.peek().typ == etComma {
		c.advance() // skip ,
		arg, _, err := c.parseExpr()
		if err != nil {
			return nil, err
		}
		args = append(args, arg)
	}

	return args, nil
}

// ensureBool 确保表达式结果为 bool 类型的 Go 代码
func (c *exprCompiler) ensureBool(code string, isBool bool) string {
	if isBool {
		return code
	}
	return fmt.Sprintf("%sToBool(%s)", c.pkg, code)
}

// ==================== 公开编译接口 ====================

// CompileExprToGo 将模板表达式编译为 Go 代码字符串。
// pkgPrefix: "gosql." (外部包) 或 "" (gosql包内)
// 返回: Go代码, 是否是bool类型, 错误
func CompileExprToGo(expr string, pkgPrefix string) (goCode string, isBool bool, err error) {
	expr = strings.TrimSpace(expr)
	if expr == "" {
		return "false", true, nil
	}

	tokens, err := tokenizeExpr(expr)
	if err != nil {
		return "", false, fmt.Errorf("tokenize error in %q: %w", expr, err)
	}

	c := &exprCompiler{tokens: tokens, pos: 0, pkg: pkgPrefix}
	goCode, isBool, err = c.parseExpr()
	if err != nil {
		return "", false, fmt.Errorf("parse error in %q: %w", expr, err)
	}

	// 确保所有 token 都被消费
	if c.current().typ != etEOF {
		return "", false, fmt.Errorf("unexpected token %q in %q", c.current().value, expr)
	}

	return goCode, isBool, nil
}

// CompileConditionToGo 将条件表达式编译为 bool 类型的 Go 代码。
func CompileConditionToGo(expr string, pkgPrefix string) (string, error) {
	code, isBool, err := CompileExprToGo(expr, pkgPrefix)
	if err != nil {
		return "", err
	}
	if !isBool {
		return pkgPrefix + "ToBool(" + code + ")", nil
	}
	return code, nil
}

// CompileValueExprToGo 将值表达式编译为 interface{} 类型的 Go 代码。
func CompileValueExprToGo(expr string, pkgPrefix string) (string, error) {
	code, _, err := CompileExprToGo(expr, pkgPrefix)
	if err != nil {
		return "", err
	}
	return code, nil
}

// ==================== 解析函数调用表达式 ====================
// 专用于解析 @funcName(args) {} 中的函数表达式，返回函数名和参数列表

type FuncCallInfo struct {
	FuncName string
	Args     []string // 编译后的 Go 代码参数
}

// ParseFuncCallExpr 解析函数调用表达式，返回函数名和已编译的参数。
// 例如: trim("and") → FuncName="trim", Args=[`"and"`]
func ParseFuncCallExpr(expr string, pkgPrefix string) (*FuncCallInfo, error) {
	expr = strings.TrimSpace(expr)
	tokens, err := tokenizeExpr(expr)
	if err != nil {
		return nil, fmt.Errorf("tokenize func expr %q: %w", expr, err)
	}

	if len(tokens) < 2 || tokens[0].typ != etIdent {
		return nil, fmt.Errorf("expected function call expression, got %q", expr)
	}

	funcName := tokens[0].value

	// 可能没有括号: 裸函数名
	if tokens[1].typ == etEOF {
		return &FuncCallInfo{FuncName: funcName}, nil
	}

	if tokens[1].typ != etLParen {
		return nil, fmt.Errorf("expected '(' after function name in %q", expr)
	}

	c := &exprCompiler{tokens: tokens, pos: 1, pkg: pkgPrefix}
	c.advance() // skip (
	args, err := c.parseArgs()
	if err != nil {
		return nil, fmt.Errorf("parse func args in %q: %w", expr, err)
	}
	if _, err := c.expect(etRParen); err != nil {
		return nil, fmt.Errorf("missing closing paren in %q", expr)
	}

	return &FuncCallInfo{FuncName: funcName, Args: args}, nil
}

// ==================== 解析 for 语句 ====================

// ForInitInfo 解析 for 循环的初始化语句
// 例: "i := 0" → VarName="i", ValueCode="0"
type ForInitInfo struct {
	VarName   string
	ValueCode string
}

func ParseForInit(initExpr string, pkgPrefix string) (*ForInitInfo, error) {
	initExpr = strings.TrimSpace(initExpr)
	// pattern: varName := expr
	parts := strings.SplitN(initExpr, ":=", 2)
	if len(parts) != 2 {
		return nil, fmt.Errorf("expected ':=' in for init: %q", initExpr)
	}
	varName := strings.TrimSpace(parts[0])
	valueExpr := strings.TrimSpace(parts[1])

	valueCode, _, err := CompileExprToGo(valueExpr, pkgPrefix)
	if err != nil {
		return nil, fmt.Errorf("compile for init value %q: %w", valueExpr, err)
	}
	return &ForInitInfo{VarName: varName, ValueCode: valueCode}, nil
}

// ForPostInfo 解析 for 循环的后置语句
// 例: "i++" → VarName="i", Code="gosql.Add(ctx.MustGet(\"i\"), 1)"
type ForPostInfo struct {
	VarName string
	Code    string
}

func ParseForPost(postExpr string, pkgPrefix string) (*ForPostInfo, error) {
	postExpr = strings.TrimSpace(postExpr)

	// i++
	if strings.HasSuffix(postExpr, "++") {
		varName := strings.TrimSuffix(postExpr, "++")
		varName = strings.TrimSpace(varName)
		return &ForPostInfo{
			VarName: varName,
			Code:    fmt.Sprintf("%sAdd(ctx.MustGet(%q), 1)", pkgPrefix, varName),
		}, nil
	}

	// i--
	if strings.HasSuffix(postExpr, "--") {
		varName := strings.TrimSuffix(postExpr, "--")
		varName = strings.TrimSpace(varName)
		return &ForPostInfo{
			VarName: varName,
			Code:    fmt.Sprintf("%sSub(ctx.MustGet(%q), 1)", pkgPrefix, varName),
		}, nil
	}

	// i += expr
	if idx := strings.Index(postExpr, "+="); idx > 0 {
		varName := strings.TrimSpace(postExpr[:idx])
		valExpr := strings.TrimSpace(postExpr[idx+2:])
		valCode, _, err := CompileExprToGo(valExpr, pkgPrefix)
		if err != nil {
			return nil, fmt.Errorf("compile for post value: %w", err)
		}
		return &ForPostInfo{
			VarName: varName,
			Code:    fmt.Sprintf("%sAdd(ctx.MustGet(%q), %s)", pkgPrefix, varName, valCode),
		}, nil
	}

	// i -= expr
	if idx := strings.Index(postExpr, "-="); idx > 0 {
		varName := strings.TrimSpace(postExpr[:idx])
		valExpr := strings.TrimSpace(postExpr[idx+2:])
		valCode, _, err := CompileExprToGo(valExpr, pkgPrefix)
		if err != nil {
			return nil, fmt.Errorf("compile for post value: %w", err)
		}
		return &ForPostInfo{
			VarName: varName,
			Code:    fmt.Sprintf("%sSub(ctx.MustGet(%q), %s)", pkgPrefix, varName, valCode),
		}, nil
	}

	return nil, fmt.Errorf("unsupported for post statement: %q", postExpr)
}

// ParseForRange 解析 for range 语句
// 例: "i, v := range ids" → IndexVar="i", ValueVar="v", RangeCode="ctx.MustGet(\"ids\")"
type ForRangeInfo struct {
	IndexVar  string
	ValueVar  string
	RangeCode string
}

func ParseForRange(rangeExpr string, pkgPrefix string) (*ForRangeInfo, error) {
	rangeExpr = strings.TrimSpace(rangeExpr)

	// pattern: [indexVar,] valueVar := range expr
	colonIdx := strings.Index(rangeExpr, ":=")
	if colonIdx < 0 {
		return nil, fmt.Errorf("expected ':=' in for range: %q", rangeExpr)
	}

	varsStr := strings.TrimSpace(rangeExpr[:colonIdx])
	restStr := strings.TrimSpace(rangeExpr[colonIdx+2:])

	if !strings.HasPrefix(restStr, "range") {
		return nil, fmt.Errorf("expected 'range' keyword: %q", rangeExpr)
	}
	rangeTarget := strings.TrimSpace(restStr[5:])

	// 解析变量: "i, v" 或 "_,v" 或 "v"
	var indexVar, valueVar string
	vars := strings.SplitN(varsStr, ",", 2)
	if len(vars) == 2 {
		indexVar = strings.TrimSpace(vars[0])
		valueVar = strings.TrimSpace(vars[1])
	} else {
		indexVar = "_"
		valueVar = strings.TrimSpace(vars[0])
	}

	// 编译 range 目标表达式
	rangeCode, _, err := CompileExprToGo(rangeTarget, pkgPrefix)
	if err != nil {
		return nil, fmt.Errorf("compile range target %q: %w", rangeTarget, err)
	}

	return &ForRangeInfo{
		IndexVar:  indexVar,
		ValueVar:  valueVar,
		RangeCode: rangeCode,
	}, nil
}

// ==================== 辅助函数 ====================

func isIdentStart(ch byte) bool {
	return ch >= 'a' && ch <= 'z' || ch >= 'A' && ch <= 'Z' || ch == '_' || ch > 127 && unicode.IsLetter(rune(ch))
}

func isIdentPart(ch byte) bool {
	return isIdentStart(ch) || ch >= '0' && ch <= '9'
}

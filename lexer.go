package gosql

import (
	"fmt"
	"strings"
	"unicode"
)

// TokenType 表示 token 类型
type TokenType int

const (
	TOKEN_EOF           TokenType = iota
	TOKEN_TEXT                    // 普通 SQL 文本
	TOKEN_VAR                     // @变量名 - 输出 ? 和参数
	TOKEN_VAR_COND                // @变量名? - 条件控制的变量
	TOKEN_VAR_EXPR                // @ expr @ - 复杂表达式输出 ? 和参数
	TOKEN_VAR_EXPR_COND           // @ expr @? - 条件控制的表达式
	TOKEN_RAW                     // @=变量名 - 直接输出值
	TOKEN_RAW_COND                // @=变量名? - 条件控制的直接输出
	TOKEN_RAW_EXPR                // @= expr @ - 复杂表达式直接输出值
	TOKEN_RAW_EXPR_COND           // @= expr @? - 条件控制的表达式直接输出
	TOKEN_IF                      // @if
	TOKEN_ELSE_IF                 // } else if
	TOKEN_ELSE                    // } else {
	TOKEN_FOR                     // @for
	TOKEN_LBRACE                  // {
	TOKEN_RBRACE                  // }
	TOKEN_CODE                    // @{} 直接的 Go 代码
	TOKEN_USE                     // @use
	TOKEN_DEFINE                  // @define
	TOKEN_COVER                   // @cover
)

// Token 表示一个词法单元
type Token struct {
	Type    TokenType
	Value   string // token 的值（变量名、表达式、SQL 文本等）
	Line    int    // 行号
	Column  int    // 列号
	Context string // 上下文片段（用于错误提示）
}

func (t TokenType) String() string {
	switch t {
	case TOKEN_EOF:
		return "EOF"
	case TOKEN_TEXT:
		return "TEXT"
	case TOKEN_VAR:
		return "VAR"
	case TOKEN_VAR_COND:
		return "VAR_COND"
	case TOKEN_VAR_EXPR:
		return "VAR_EXPR"
	case TOKEN_VAR_EXPR_COND:
		return "VAR_EXPR_COND"
	case TOKEN_RAW:
		return "RAW"
	case TOKEN_RAW_COND:
		return "RAW_COND"
	case TOKEN_RAW_EXPR:
		return "RAW_EXPR"
	case TOKEN_RAW_EXPR_COND:
		return "RAW_EXPR_COND"
	case TOKEN_IF:
		return "IF"
	case TOKEN_ELSE_IF:
		return "ELSE_IF"
	case TOKEN_ELSE:
		return "ELSE"
	case TOKEN_FOR:
		return "FOR"
	case TOKEN_LBRACE:
		return "LBRACE"
	case TOKEN_RBRACE:
		return "RBRACE"
	case TOKEN_CODE:
		return "CODE"
	case TOKEN_USE:
		return "USE"
	case TOKEN_DEFINE:
		return "DEFINE"
	case TOKEN_COVER:
		return "COVER"
	default:
		return "UNKNOWN"
	}
}

// Lexer SQL 模板词法分析器
type Lexer struct {
	input  string
	pos    int
	line   int
	column int
	tokens []Token
}

// NewLexer 创建词法分析器
func NewLexer(input string) *Lexer {
	return &Lexer{
		input:  input,
		pos:    0,
		line:   1,
		column: 1,
	}
}

// Tokenize 执行词法分析
func (l *Lexer) Tokenize() ([]Token, error) {
	for l.pos < len(l.input) {
		if err := l.scanToken(); err != nil {
			return nil, err
		}
	}

	l.tokens = append(l.tokens, Token{
		Type:   TOKEN_EOF,
		Line:   l.line,
		Column: l.column,
	})

	return l.tokens, nil
}

// scanToken 扫描下一个 token
func (l *Lexer) scanToken() error {
	// 检查是否以 @ 开始
	if l.peek() == '@' {
		return l.scanAtToken()
	}

	// 检查是否是 } 开始（可能是 } else if 或 } else {）
	if l.peek() == '}' {
		return l.scanCloseBrace()
	}

	// 普通文本
	return l.scanText()
}

// peek 查看当前字符
func (l *Lexer) peek() byte {
	if l.pos >= len(l.input) {
		return 0
	}
	return l.input[l.pos]
}

// peekN 查看后面 n 个字符
func (l *Lexer) peekN(n int) string {
	end := l.pos + n
	if end > len(l.input) {
		end = len(l.input)
	}
	return l.input[l.pos:end]
}

// advance 前进一个字符
func (l *Lexer) advance() byte {
	ch := l.input[l.pos]
	l.pos++
	if ch == '\n' {
		l.line++
		l.column = 1
	} else {
		l.column++
	}
	return ch
}

// skipWhitespace 跳过空白字符（但不包括换行符，在某些情况下）
func (l *Lexer) skipWhitespace() {
	for l.pos < len(l.input) && (l.peek() == ' ' || l.peek() == '\t') {
		l.advance()
	}
}

// skipAllWhitespace 跳过所有空白字符
func (l *Lexer) skipAllWhitespace() {
	for l.pos < len(l.input) && unicode.IsSpace(rune(l.peek())) {
		l.advance()
	}
}

// scanAtToken 扫描 @ 开头的 token
func (l *Lexer) scanAtToken() error {
	startLine := l.line
	startColumn := l.column
	l.advance() // 跳过 @

	// 检查 @=
	if l.peek() == '=' {
		l.advance()
		return l.scanRawToken(startLine, startColumn)
	}

	// 检查 @{} 直接代码块
	if l.peek() == '{' {
		return l.scanCodeBlock(startLine, startColumn)
	}

	// 检查 @ expr @ 表达式（@ 后面是空格开始的表达式）
	if l.peek() == ' ' || l.peek() == '\t' {
		l.skipWhitespace()
		return l.scanVarExpr(startLine, startColumn)
	}

	// 读取关键字或变量名
	word := l.readWord()

	switch word {
	case "if":
		return l.scanIfToken(startLine, startColumn)
	case "for":
		return l.scanForToken(startLine, startColumn)
	case "use":
		return l.scanUseToken(startLine, startColumn)
	case "define":
		return l.scanDefineToken(startLine, startColumn)
	case "cover":
		return l.scanCoverToken(startLine, startColumn)
	default:
		// 检查是否以 ? 结尾（条件控制）
		tokenType := TOKEN_VAR
		if l.peek() == '?' {
			l.advance()
			tokenType = TOKEN_VAR_COND
		}
		l.tokens = append(l.tokens, Token{
			Type:    tokenType,
			Value:   word,
			Line:    startLine,
			Column:  startColumn,
			Context: l.getContext(startLine),
		})
		return nil
	}
}

// readWord 读取一个单词（字母、数字、下划线）
func (l *Lexer) readWord() string {
	var sb strings.Builder
	for l.pos < len(l.input) {
		ch := l.peek()
		if unicode.IsLetter(rune(ch)) || unicode.IsDigit(rune(ch)) || ch == '_' {
			sb.WriteByte(l.advance())
		} else {
			break
		}
	}
	return sb.String()
}

// scanVarExpr 扫描 @ expr @ 表达式
func (l *Lexer) scanVarExpr(startLine, startColumn int) error {
	expr, err := l.readUntilAt()
	if err != nil {
		return err
	}

	// 检查是否以 ? 结尾（条件控制）
	tokenType := TOKEN_VAR_EXPR
	if l.peek() == '?' {
		l.advance()
		tokenType = TOKEN_VAR_EXPR_COND
	}

	l.tokens = append(l.tokens, Token{
		Type:    tokenType,
		Value:   strings.TrimSpace(expr),
		Line:    startLine,
		Column:  startColumn,
		Context: l.getContext(startLine),
	})
	return nil
}

// scanRawToken 扫描 @= 开头的 token
func (l *Lexer) scanRawToken(startLine, startColumn int) error {
	// 检查是否是表达式（后面有空格）
	if l.peek() == ' ' || l.peek() == '\t' {
		l.skipWhitespace()
		expr, err := l.readUntilAt()
		if err != nil {
			return err
		}

		// 检查是否以 ? 结尾（条件控制）
		tokenType := TOKEN_RAW_EXPR
		if l.peek() == '?' {
			l.advance()
			tokenType = TOKEN_RAW_EXPR_COND
		}

		l.tokens = append(l.tokens, Token{
			Type:    tokenType,
			Value:   strings.TrimSpace(expr),
			Line:    startLine,
			Column:  startColumn,
			Context: l.getContext(startLine),
		})
		return nil
	}

	// 普通变量名
	word := l.readWord()

	// 检查是否以 @ 结尾（@=var@ 形式）
	if l.peek() == '@' {
		l.advance() // 跳过结束的 @
	}

	// 检查是否以 ? 结尾（条件控制）
	tokenType := TOKEN_RAW
	if l.peek() == '?' {
		l.advance()
		tokenType = TOKEN_RAW_COND
	}

	l.tokens = append(l.tokens, Token{
		Type:    tokenType,
		Value:   word,
		Line:    startLine,
		Column:  startColumn,
		Context: l.getContext(startLine),
	})
	return nil
}

// readUntilAt 读取直到遇到 @
func (l *Lexer) readUntilAt() (string, error) {
	var sb strings.Builder
	startLine := l.line

	for l.pos < len(l.input) {
		if l.peek() == '@' {
			l.advance() // 跳过结束的 @
			return sb.String(), nil
		}
		sb.WriteByte(l.advance())
	}

	return "", fmt.Errorf("line %d: unclosed expression, expected '@' to close the expression", startLine)
}

// scanCodeBlock 扫描 @{} 代码块
func (l *Lexer) scanCodeBlock(startLine, startColumn int) error {
	l.advance() // 跳过 {

	code, err := l.readUntilMatchingBrace()
	if err != nil {
		return err
	}

	l.tokens = append(l.tokens, Token{
		Type:    TOKEN_CODE,
		Value:   strings.TrimSpace(code),
		Line:    startLine,
		Column:  startColumn,
		Context: l.getContext(startLine),
	})
	return nil
}

// scanIfToken 扫描 @if 语句
func (l *Lexer) scanIfToken(startLine, startColumn int) error {
	l.skipWhitespace()

	// 读取条件表达式直到 {
	condition, err := l.readUntilBrace()
	if err != nil {
		return err
	}

	l.tokens = append(l.tokens, Token{
		Type:    TOKEN_IF,
		Value:   strings.TrimSpace(condition),
		Line:    startLine,
		Column:  startColumn,
		Context: l.getContext(startLine),
	})

	l.tokens = append(l.tokens, Token{
		Type:   TOKEN_LBRACE,
		Line:   l.line,
		Column: l.column,
	})
	l.advance() // 跳过 {

	return nil
}

// scanForToken 扫描 @for 语句
func (l *Lexer) scanForToken(startLine, startColumn int) error {
	l.skipWhitespace()

	// 读取 for 表达式直到 {
	expr, err := l.readUntilBrace()
	if err != nil {
		return err
	}

	l.tokens = append(l.tokens, Token{
		Type:    TOKEN_FOR,
		Value:   strings.TrimSpace(expr),
		Line:    startLine,
		Column:  startColumn,
		Context: l.getContext(startLine),
	})

	l.tokens = append(l.tokens, Token{
		Type:   TOKEN_LBRACE,
		Line:   l.line,
		Column: l.column,
	})
	l.advance() // 跳过 {

	return nil
}

// scanUseToken 扫描 @use 语句
func (l *Lexer) scanUseToken(startLine, startColumn int) error {
	l.skipWhitespace()

	// 读取 use 路径直到 {
	path, err := l.readUntilBrace()
	if err != nil {
		return err
	}

	l.tokens = append(l.tokens, Token{
		Type:    TOKEN_USE,
		Value:   strings.TrimSpace(path),
		Line:    startLine,
		Column:  startColumn,
		Context: l.getContext(startLine),
	})

	l.tokens = append(l.tokens, Token{
		Type:   TOKEN_LBRACE,
		Line:   l.line,
		Column: l.column,
	})
	l.advance() // 跳过 {

	return nil
}

// scanDefineToken 扫描 @define 语句
func (l *Lexer) scanDefineToken(startLine, startColumn int) error {
	l.skipWhitespace()

	// 读取 define 名称直到 {
	name, err := l.readUntilBrace()
	if err != nil {
		return err
	}

	l.tokens = append(l.tokens, Token{
		Type:    TOKEN_DEFINE,
		Value:   strings.TrimSpace(name),
		Line:    startLine,
		Column:  startColumn,
		Context: l.getContext(startLine),
	})

	l.tokens = append(l.tokens, Token{
		Type:   TOKEN_LBRACE,
		Line:   l.line,
		Column: l.column,
	})
	l.advance() // 跳过 {

	return nil
}

// scanCoverToken 扫描 @cover 语句
func (l *Lexer) scanCoverToken(startLine, startColumn int) error {
	l.skipWhitespace()

	// 读取 cover 名称直到 {
	name, err := l.readUntilBrace()
	if err != nil {
		return err
	}

	l.tokens = append(l.tokens, Token{
		Type:    TOKEN_COVER,
		Value:   strings.TrimSpace(name),
		Line:    startLine,
		Column:  startColumn,
		Context: l.getContext(startLine),
	})

	l.tokens = append(l.tokens, Token{
		Type:   TOKEN_LBRACE,
		Line:   l.line,
		Column: l.column,
	})
	l.advance() // 跳过 {

	return nil
}

// scanCloseBrace 扫描 } 及其后续（可能是 else if 或 else）
func (l *Lexer) scanCloseBrace() error {
	startLine := l.line
	startColumn := l.column
	l.advance() // 跳过 }

	// 保存当前位置，尝试匹配 else
	savedPos := l.pos
	savedLine := l.line
	savedColumn := l.column

	l.skipAllWhitespace()

	// 检查是否是 else
	if l.peekN(4) == "else" {
		l.pos += 4
		l.column += 4
		l.skipWhitespace()

		// 检查是否是 else if
		if l.peekN(2) == "if" {
			l.pos += 2
			l.column += 2
			l.skipWhitespace()

			// 读取条件
			condition, err := l.readUntilBrace()
			if err != nil {
				return err
			}

			l.tokens = append(l.tokens, Token{
				Type:    TOKEN_ELSE_IF,
				Value:   strings.TrimSpace(condition),
				Line:    startLine,
				Column:  startColumn,
				Context: l.getContext(startLine),
			})

			l.tokens = append(l.tokens, Token{
				Type:   TOKEN_LBRACE,
				Line:   l.line,
				Column: l.column,
			})
			l.advance() // 跳过 {
			return nil
		}

		// 只是 else
		if l.peek() == '{' {
			l.tokens = append(l.tokens, Token{
				Type:   TOKEN_ELSE,
				Line:   startLine,
				Column: startColumn,
			})

			l.tokens = append(l.tokens, Token{
				Type:   TOKEN_LBRACE,
				Line:   l.line,
				Column: l.column,
			})
			l.advance() // 跳过 {
			return nil
		}
	}

	// 不是 else，恢复位置，只输出 }
	l.pos = savedPos
	l.line = savedLine
	l.column = savedColumn

	l.tokens = append(l.tokens, Token{
		Type:   TOKEN_RBRACE,
		Line:   startLine,
		Column: startColumn,
	})
	return nil
}

// scanText 扫描普通文本
func (l *Lexer) scanText() error {
	startLine := l.line
	startColumn := l.column
	var sb strings.Builder

	for l.pos < len(l.input) {
		ch := l.peek()
		if ch == '@' || ch == '}' {
			break
		}
		sb.WriteByte(l.advance())
	}

	text := sb.String()
	if len(text) > 0 {
		l.tokens = append(l.tokens, Token{
			Type:   TOKEN_TEXT,
			Value:  text,
			Line:   startLine,
			Column: startColumn,
		})
	}

	return nil
}

// readUntilBrace 读取直到遇到 {
func (l *Lexer) readUntilBrace() (string, error) {
	var sb strings.Builder
	startLine := l.line

	for l.pos < len(l.input) {
		if l.peek() == '{' {
			return sb.String(), nil
		}
		sb.WriteByte(l.advance())
	}

	return "", fmt.Errorf("line %d: expected '{' but reached end of input", startLine)
}

// readUntilMatchingBrace 读取直到匹配的 }
func (l *Lexer) readUntilMatchingBrace() (string, error) {
	var sb strings.Builder
	startLine := l.line
	depth := 1

	for l.pos < len(l.input) && depth > 0 {
		ch := l.peek()
		if ch == '{' {
			depth++
		} else if ch == '}' {
			depth--
			if depth == 0 {
				l.advance() // 跳过结束的 }
				return sb.String(), nil
			}
		}
		sb.WriteByte(l.advance())
	}

	return "", fmt.Errorf("line %d: unclosed brace, expected '}' to close the code block", startLine)
}

// getContext 获取指定行的上下文
func (l *Lexer) getContext(line int) string {
	lines := strings.Split(l.input, "\n")
	if line <= 0 || line > len(lines) {
		return ""
	}

	start := line - 2
	if start < 0 {
		start = 0
	}
	end := line + 1
	if end > len(lines) {
		end = len(lines)
	}

	var sb strings.Builder
	for i := start; i < end; i++ {
		if i == line-1 {
			sb.WriteString(">>> ")
		} else {
			sb.WriteString("    ")
		}
		sb.WriteString(lines[i])
		sb.WriteString("\n")
	}
	return sb.String()
}

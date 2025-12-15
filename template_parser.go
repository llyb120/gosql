package gosql

import (
	"fmt"
	"strings"
)

// TemplateParser SQL 模板解析器
type TemplateParser struct {
	tokens []Token
	pos    int
}

// NewTemplateParser 创建模板解析器
func NewTemplateParser(tokens []Token) *TemplateParser {
	return &TemplateParser{
		tokens: tokens,
		pos:    0,
	}
}

// Parse 解析 tokens 为 AST
func (p *TemplateParser) Parse() (*TemplateAST, error) {
	nodes, err := p.parseNodes()
	if err != nil {
		return nil, err
	}

	return &TemplateAST{
		Nodes: nodes,
	}, nil
}

// parseNodes 解析节点列表
func (p *TemplateParser) parseNodes() ([]Node, error) {
	var nodes []Node

	for !p.isAtEnd() && !p.check(TOKEN_RBRACE) && !p.check(TOKEN_ELSE_IF) && !p.check(TOKEN_ELSE) {
		node, err := p.parseNode()
		if err != nil {
			return nil, err
		}
		if node != nil {
			nodes = append(nodes, node)
		}
	}

	return nodes, nil
}

// parseNode 解析单个节点
func (p *TemplateParser) parseNode() (Node, error) {
	token := p.peek()

	switch token.Type {
	case TOKEN_EOF:
		return nil, nil

	case TOKEN_TEXT:
		p.advance()
		return &TextNode{Text: token.Value}, nil

	case TOKEN_VAR:
		p.advance()
		return &VarNode{Name: token.Value, Conditional: false}, nil

	case TOKEN_VAR_COND:
		p.advance()
		return &VarNode{Name: token.Value, Conditional: true}, nil

	case TOKEN_VAR_EXPR:
		p.advance()
		return &VarExprNode{Expr: token.Value, Conditional: false}, nil

	case TOKEN_VAR_EXPR_COND:
		p.advance()
		return &VarExprNode{Expr: token.Value, Conditional: true}, nil

	case TOKEN_RAW:
		p.advance()
		return &RawNode{Name: token.Value, Conditional: false}, nil

	case TOKEN_RAW_COND:
		p.advance()
		return &RawNode{Name: token.Value, Conditional: true}, nil

	case TOKEN_RAW_EXPR:
		p.advance()
		return &RawExprNode{Expr: token.Value, Conditional: false}, nil

	case TOKEN_RAW_EXPR_COND:
		p.advance()
		return &RawExprNode{Expr: token.Value, Conditional: true}, nil

	case TOKEN_IF:
		return p.parseIf()

	case TOKEN_FOR:
		return p.parseFor()

	case TOKEN_CODE:
		p.advance()
		return &CodeNode{Code: token.Value}, nil

	case TOKEN_USE:
		return p.parseUse()

	case TOKEN_DEFINE:
		return p.parseDefine()

	case TOKEN_COVER:
		return p.parseCover()

	case TOKEN_FUNC_BLOCK:
		return p.parseFuncBlock()

	case TOKEN_LBRACE:
		// 跳过孤立的 {
		p.advance()
		return nil, nil

	case TOKEN_RBRACE:
		// } 会在外层处理
		return nil, nil

	case TOKEN_ELSE_IF, TOKEN_ELSE:
		// 这些会在 parseIf 中处理
		return nil, nil

	default:
		return nil, fmt.Errorf("line %d, column %d: unexpected token type %s\n%s",
			token.Line, token.Column, token.Type.String(), token.Context)
	}
}

// parseIf 解析 if 语句
func (p *TemplateParser) parseIf() (Node, error) {
	token := p.advance() // 消费 IF token
	condition := token.Value

	// 期望 {
	if !p.match(TOKEN_LBRACE) {
		return nil, fmt.Errorf("line %d: expected '{' after if condition", token.Line)
	}

	// 解析 if 体
	body, err := p.parseNodes()
	if err != nil {
		return nil, err
	}

	ifNode := &IfNode{
		Condition: condition,
		Body:      body,
	}

	// 解析 else if / else
	for {
		if p.check(TOKEN_ELSE_IF) {
			elseIfToken := p.advance()

			// 期望 {
			if !p.match(TOKEN_LBRACE) {
				return nil, fmt.Errorf("line %d: expected '{' after else if condition", elseIfToken.Line)
			}

			elseIfBody, err := p.parseNodes()
			if err != nil {
				return nil, err
			}

			ifNode.ElseIf = append(ifNode.ElseIf, &ElseIfNode{
				Condition: elseIfToken.Value,
				Body:      elseIfBody,
			})
		} else if p.check(TOKEN_ELSE) {
			p.advance()

			// 期望 {
			if !p.match(TOKEN_LBRACE) {
				return nil, fmt.Errorf("line %d: expected '{' after else", p.peek().Line)
			}

			elseBody, err := p.parseNodes()
			if err != nil {
				return nil, err
			}

			ifNode.Else = &ElseNode{
				Body: elseBody,
			}

			// else 后面没有其他分支
			break
		} else {
			break
		}
	}

	// 期望最后的 }
	if !p.match(TOKEN_RBRACE) {
		return nil, fmt.Errorf("line %d: expected '}' to close if statement", p.peek().Line)
	}

	return ifNode, nil
}

// parseFor 解析 for 语句
func (p *TemplateParser) parseFor() (Node, error) {
	token := p.advance() // 消费 FOR token
	expr := token.Value

	// 期望 {
	if !p.match(TOKEN_LBRACE) {
		return nil, fmt.Errorf("line %d: expected '{' after for expression", token.Line)
	}

	// 解析 for 体
	body, err := p.parseNodes()
	if err != nil {
		return nil, err
	}

	// 期望 }
	if !p.match(TOKEN_RBRACE) {
		return nil, fmt.Errorf("line %d: expected '}' to close for statement", p.peek().Line)
	}

	return &ForNode{
		Expr: expr,
		Body: body,
	}, nil
}

// parseUse 解析 use 语句
func (p *TemplateParser) parseUse() (Node, error) {
	token := p.advance() // 消费 USE token
	path := token.Value

	// 期望 {
	if !p.match(TOKEN_LBRACE) {
		return nil, fmt.Errorf("line %d: expected '{' after use path", token.Line)
	}

	useNode := &UseNode{
		Path: path,
	}

	// 解析 cover 块
	for !p.isAtEnd() && !p.check(TOKEN_RBRACE) {
		// 跳过文本（空白）
		if p.check(TOKEN_TEXT) {
			p.advance()
			continue
		}

		if p.check(TOKEN_COVER) {
			cover, err := p.parseCover()
			if err != nil {
				return nil, err
			}
			useNode.Covers = append(useNode.Covers, cover.(*CoverNode))
		} else {
			break
		}
	}

	// 期望 }
	if !p.match(TOKEN_RBRACE) {
		return nil, fmt.Errorf("line %d: expected '}' to close use statement", p.peek().Line)
	}

	return useNode, nil
}

// parseDefine 解析 define 语句
func (p *TemplateParser) parseDefine() (Node, error) {
	token := p.advance() // 消费 DEFINE token
	name := token.Value

	// 期望 {
	if !p.match(TOKEN_LBRACE) {
		return nil, fmt.Errorf("line %d: expected '{' after define name", token.Line)
	}

	// 解析 define 体
	body, err := p.parseNodes()
	if err != nil {
		return nil, err
	}

	// 期望 }
	if !p.match(TOKEN_RBRACE) {
		return nil, fmt.Errorf("line %d: expected '}' to close define statement", p.peek().Line)
	}

	return &DefineNode{
		Name: name,
		Body: body,
	}, nil
}

// parseCover 解析 cover 语句
func (p *TemplateParser) parseCover() (Node, error) {
	token := p.advance() // 消费 COVER token
	name := token.Value

	// 期望 {
	if !p.match(TOKEN_LBRACE) {
		return nil, fmt.Errorf("line %d: expected '{' after cover name", token.Line)
	}

	// 解析 cover 体
	body, err := p.parseNodes()
	if err != nil {
		return nil, err
	}

	// 期望 }
	if !p.match(TOKEN_RBRACE) {
		return nil, fmt.Errorf("line %d: expected '}' to close cover statement", p.peek().Line)
	}

	return &CoverNode{
		Name: name,
		Body: body,
	}, nil
}

// parseFuncBlock 解析函数块 @ func() {}
func (p *TemplateParser) parseFuncBlock() (Node, error) {
	token := p.advance() // 消费 FUNC_BLOCK token

	// token.Value 格式为 "funcExpr|blockContent"
	parts := strings.SplitN(token.Value, "|", 2)
	funcExpr := parts[0]
	blockContent := ""
	if len(parts) > 1 {
		blockContent = parts[1]
	}

	// 解析块内容为节点
	var bodyNodes []Node
	if blockContent != "" {
		lexer := NewLexer(blockContent)
		tokens, err := lexer.Tokenize()
		if err != nil {
			return nil, fmt.Errorf("line %d: error parsing func block: %w", token.Line, err)
		}
		subParser := NewTemplateParser(tokens)
		ast, err := subParser.Parse()
		if err != nil {
			return nil, fmt.Errorf("line %d: error parsing func block: %w", token.Line, err)
		}
		bodyNodes = ast.Nodes
	}

	return &FuncBlockNode{
		FuncExpr: funcExpr,
		Body:     bodyNodes,
	}, nil
}

// 辅助方法

func (p *TemplateParser) peek() Token {
	if p.pos >= len(p.tokens) {
		return Token{Type: TOKEN_EOF}
	}
	return p.tokens[p.pos]
}

func (p *TemplateParser) advance() Token {
	token := p.peek()
	p.pos++
	return token
}

func (p *TemplateParser) check(tokenType TokenType) bool {
	return p.peek().Type == tokenType
}

func (p *TemplateParser) match(tokenType TokenType) bool {
	if p.check(tokenType) {
		p.advance()
		return true
	}
	return false
}

func (p *TemplateParser) isAtEnd() bool {
	return p.peek().Type == TOKEN_EOF
}

// ParseTemplate 便捷函数：从 SQL 模板内容解析为 AST
func ParseTemplate(content string) (*TemplateAST, error) {
	lexer := NewLexer(content)
	tokens, err := lexer.Tokenize()
	if err != nil {
		return nil, err
	}

	parser := NewTemplateParser(tokens)
	return parser.Parse()
}


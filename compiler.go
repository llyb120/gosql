package gosql

import (
	"fmt"
	"reflect"
	"strconv"
	"strings"
)

// Compiler 将 AST 编译为 Go 代码
type Compiler struct {
	code       strings.Builder
	indent     int
	varCounter int
}

// NewCompiler 创建编译器
func NewCompiler() *Compiler {
	return &Compiler{
		indent:     0,
		varCounter: 0,
	}
}

// Compile 编译 AST 为 Go 代码
func (c *Compiler) Compile(ast *TemplateAST) (string, error) {
	c.code.Reset()

	// 生成代码头部
	c.writeLine("package main")
	c.writeLine("")
	c.writeLine("import \"reflect\"")
	c.writeLine("")
	
	// 声明输出变量
	c.writeLine("var __sql__ strings.Builder")
	c.writeLine("var __args__ []interface{}")
	c.writeLine("")
	
	// 生成辅助函数
	c.generateHelperFunctions()
	c.writeLine("")

	// 编译节点
	if err := c.compileNodes(ast.Nodes); err != nil {
		return "", err
	}

	return c.code.String(), nil
}

// generateHelperFunctions 生成辅助函数
func (c *Compiler) generateHelperFunctions() {
	// __appendArg__ 函数：添加参数（支持数组展开）
	c.writeLine("func __appendArg__(v interface{}) {")
	c.indent++
	c.writeLine("rv := reflect.ValueOf(v)")
	c.writeLine("if rv.Kind() == reflect.Slice || rv.Kind() == reflect.Array {")
	c.indent++
	c.writeLine("for i := 0; i < rv.Len(); i++ {")
	c.indent++
	c.writeLine("if i > 0 {")
	c.indent++
	c.writeLine("__sql__.WriteString(\", \")")
	c.indent--
	c.writeLine("}")
	c.writeLine("__sql__.WriteString(\"?\")")
	c.writeLine("__args__ = append(__args__, rv.Index(i).Interface())")
	c.indent--
	c.writeLine("}")
	c.indent--
	c.writeLine("} else {")
	c.indent++
	c.writeLine("__sql__.WriteString(\"?\")")
	c.writeLine("__args__ = append(__args__, v)")
	c.indent--
	c.writeLine("}")
	c.indent--
	c.writeLine("}")

	c.writeLine("")

	// __appendRaw__ 函数：直接添加值
	c.writeLine("func __appendRaw__(v interface{}) {")
	c.indent++
	c.writeLine("__sql__.WriteString(fmt.Sprintf(\"%v\", v))")
	c.indent--
	c.writeLine("}")
}

// compileNodes 编译节点列表
func (c *Compiler) compileNodes(nodes []Node) error {
	for _, node := range nodes {
		if err := c.compileNode(node); err != nil {
			return err
		}
	}
	return nil
}

// compileNode 编译单个节点
func (c *Compiler) compileNode(node Node) error {
	switch n := node.(type) {
	case *TextNode:
		return c.compileText(n)
	case *VarNode:
		return c.compileVar(n)
	case *VarExprNode:
		return c.compileVarExpr(n)
	case *RawNode:
		return c.compileRaw(n)
	case *RawExprNode:
		return c.compileRawExpr(n)
	case *IfNode:
		return c.compileIf(n)
	case *ForNode:
		return c.compileFor(n)
	case *CodeNode:
		return c.compileCode(n)
	case *UseNode:
		return c.compileUse(n)
	case *DefineNode:
		return c.compileDefine(n)
	default:
		return fmt.Errorf("unknown node type: %T", node)
	}
}

// compileText 编译文本节点
func (c *Compiler) compileText(n *TextNode) error {
	// 转义字符串中的特殊字符
	escaped := strconv.Quote(n.Text)
	c.writeLine(fmt.Sprintf("__sql__.WriteString(%s)", escaped))
	return nil
}

// compileVar 编译变量节点
func (c *Compiler) compileVar(n *VarNode) error {
	c.writeLine(fmt.Sprintf("__appendArg__(%s)", n.Name))
	return nil
}

// compileVarExpr 编译变量表达式节点
func (c *Compiler) compileVarExpr(n *VarExprNode) error {
	c.writeLine(fmt.Sprintf("__appendArg__(%s)", n.Expr))
	return nil
}

// compileRaw 编译直接输出变量节点
func (c *Compiler) compileRaw(n *RawNode) error {
	c.writeLine(fmt.Sprintf("__appendRaw__(%s)", n.Name))
	return nil
}

// compileRawExpr 编译直接输出表达式节点
func (c *Compiler) compileRawExpr(n *RawExprNode) error {
	c.writeLine(fmt.Sprintf("__appendRaw__(%s)", n.Expr))
	return nil
}

// compileIf 编译 if 节点
func (c *Compiler) compileIf(n *IfNode) error {
	c.writeLine(fmt.Sprintf("if %s {", n.Condition))
	c.indent++
	if err := c.compileNodes(n.Body); err != nil {
		return err
	}
	c.indent--

	for _, elseIf := range n.ElseIf {
		c.writeLine(fmt.Sprintf("} else if %s {", elseIf.Condition))
		c.indent++
		if err := c.compileNodes(elseIf.Body); err != nil {
			return err
		}
		c.indent--
	}

	if n.Else != nil {
		c.writeLine("} else {")
		c.indent++
		if err := c.compileNodes(n.Else.Body); err != nil {
			return err
		}
		c.indent--
	}

	c.writeLine("}")
	return nil
}

// compileFor 编译 for 节点
func (c *Compiler) compileFor(n *ForNode) error {
	c.writeLine(fmt.Sprintf("for %s {", n.Expr))
	c.indent++
	if err := c.compileNodes(n.Body); err != nil {
		return err
	}
	c.indent--
	c.writeLine("}")
	return nil
}

// compileCode 编译直接代码节点
func (c *Compiler) compileCode(n *CodeNode) error {
	// 直接输出代码
	lines := strings.Split(n.Code, "\n")
	for _, line := range lines {
		c.writeLine(strings.TrimSpace(line))
	}
	return nil
}

// compileUse 编译 use 节点
func (c *Compiler) compileUse(n *UseNode) error {
	// use 会被上层处理，这里生成调用代码
	// 调用 __useTemplate__(path, covers) 函数
	c.varCounter++
	coversVar := fmt.Sprintf("__covers_%d__", c.varCounter)
	
	c.writeLine(fmt.Sprintf("%s := make(map[string]func())", coversVar))
	
	for _, cover := range n.Covers {
		c.varCounter++
		funcName := fmt.Sprintf("__cover_func_%d__", c.varCounter)
		
		// 生成 cover 函数
		c.writeLine(fmt.Sprintf("%s := func() {", funcName))
		c.indent++
		if err := c.compileNodes(cover.Body); err != nil {
			return err
		}
		c.indent--
		c.writeLine("}")
		c.writeLine(fmt.Sprintf("%s[\"%s\"] = %s", coversVar, cover.Name, funcName))
	}

	c.writeLine(fmt.Sprintf("__useTemplate__(\"%s\", %s)", n.Path, coversVar))
	return nil
}

// compileDefine 编译 define 节点
func (c *Compiler) compileDefine(n *DefineNode) error {
	// 检查是否有 cover 覆盖这个 define
	c.writeLine(fmt.Sprintf("if __cover__, __ok__ := __covers__[\"%s\"]; __ok__ {", n.Name))
	c.indent++
	c.writeLine("__cover__()")
	c.indent--
	c.writeLine("} else {")
	c.indent++
	if err := c.compileNodes(n.Body); err != nil {
		return err
	}
	c.indent--
	c.writeLine("}")
	return nil
}

// writeLine 写入一行代码
func (c *Compiler) writeLine(line string) {
	for i := 0; i < c.indent; i++ {
		c.code.WriteString("\t")
	}
	c.code.WriteString(line)
	c.code.WriteString("\n")
}

// CompileForExecution 编译为可执行的完整代码
func (c *Compiler) CompileForExecution(ast *TemplateAST, hasUse bool) (string, error) {
	c.code.Reset()
	c.varCounter = 0

	// 不需要 package 声明，goscript2 会自动处理
	
	// 编译节点
	if err := c.compileNodes(ast.Nodes); err != nil {
		return "", err
	}

	return c.code.String(), nil
}

// GenerateTemplateFunc 为模板生成函数代码
func GenerateTemplateFunc(ast *TemplateAST) (string, error) {
	compiler := NewCompiler()
	return compiler.CompileForExecution(ast, false)
}

// isSliceOrArray 判断值是否是切片或数组
func isSliceOrArray(v interface{}) bool {
	if v == nil {
		return false
	}
	rv := reflect.ValueOf(v)
	kind := rv.Kind()
	return kind == reflect.Slice || kind == reflect.Array
}

// expandArgs 展开数组参数
func expandArgs(v interface{}) (placeholders string, args []interface{}) {
	rv := reflect.ValueOf(v)
	
	if rv.Kind() == reflect.Slice || rv.Kind() == reflect.Array {
		n := rv.Len()
		ps := make([]string, n)
		args = make([]interface{}, n)
		for i := 0; i < n; i++ {
			ps[i] = "?"
			args[i] = rv.Index(i).Interface()
		}
		return strings.Join(ps, ", "), args
	}
	
	return "?", []interface{}{v}
}


package gosql

// Node 表示 AST 节点
type Node interface {
	nodeType() string
}

// TextNode 普通文本节点
type TextNode struct {
	Text string
}

func (n *TextNode) nodeType() string { return "text" }

// VarNode 变量节点 @var - 输出 ? 和参数
type VarNode struct {
	Name        string
	Conditional bool // 是否以 ? 结尾（条件控制）
}

func (n *VarNode) nodeType() string { return "var" }

// VarExprNode 变量表达式节点 @ expr @ - 输出 ? 和参数
type VarExprNode struct {
	Expr        string
	Conditional bool // 是否以 ? 结尾（条件控制）
}

func (n *VarExprNode) nodeType() string { return "var_expr" }

// RawNode 直接输出变量节点 @=var
type RawNode struct {
	Name        string
	Conditional bool // 是否以 ? 结尾（条件控制）
}

func (n *RawNode) nodeType() string { return "raw" }

// RawExprNode 直接输出表达式节点 @= expr @
type RawExprNode struct {
	Expr        string
	Conditional bool // 是否以 ? 结尾（条件控制）
}

func (n *RawExprNode) nodeType() string { return "raw_expr" }

// ConditionalLineNode 条件行节点（包含带 ? 的表达式的整行）
type ConditionalLineNode struct {
	Condition string // 条件表达式（去掉 ? 后的部分）
	LineNodes []Node // 该行的所有节点
}

func (n *ConditionalLineNode) nodeType() string { return "conditional_line" }

// IfNode if 语句节点
type IfNode struct {
	Condition string
	Body      []Node
	ElseIf    []*ElseIfNode
	Else      *ElseNode
}

func (n *IfNode) nodeType() string { return "if" }

// ElseIfNode else if 语句节点
type ElseIfNode struct {
	Condition string
	Body      []Node
}

func (n *ElseIfNode) nodeType() string { return "else_if" }

// ElseNode else 语句节点
type ElseNode struct {
	Body []Node
}

func (n *ElseNode) nodeType() string { return "else" }

// ForNode for 语句节点
type ForNode struct {
	Expr string // for 表达式（如 i := 0; i < 10; i++ 或 i, v := range arr）
	Body []Node
}

func (n *ForNode) nodeType() string { return "for" }

// CodeNode 直接 Go 代码节点 @{}
type CodeNode struct {
	Code string
}

func (n *CodeNode) nodeType() string { return "code" }

// UseNode use 语句节点
type UseNode struct {
	Path   string       // 引用路径（如 a.b 或 a.b.c）
	Covers []*CoverNode // cover 覆盖块
}

func (n *UseNode) nodeType() string { return "use" }

// DefineNode define 语句节点
type DefineNode struct {
	Name string
	Body []Node
}

func (n *DefineNode) nodeType() string { return "define" }

// CoverNode cover 语句节点
type CoverNode struct {
	Name string
	Body []Node
}

func (n *CoverNode) nodeType() string { return "cover" }

// FuncBlockNode 自定义函数块节点 @ func() {}
type FuncBlockNode struct {
	FuncExpr string // 函数表达式（如 GetName()）
	Body     []Node // 块内节点
}

func (n *FuncBlockNode) nodeType() string { return "func_block" }

// TemplateAST 模板 AST
type TemplateAST struct {
	Namespace string
	Name      string
	Nodes     []Node
}


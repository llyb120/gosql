package gosql

import (
	"fmt"
	"reflect"
	"strings"
	"unsafe"

	"github.com/llyb120/goscript2/interpreter"
)

// Query 表示 SQL 查询结果
type Query struct {
	SQL    string        // SQL 语句
	Params []interface{} // 参数列表
}

// Engine SQL 模板引擎
type Engine struct {
	store       *TemplateStore
	compiledAST map[string]*TemplateAST // 缓存编译后的 AST
	interp      *interpreter.Interpreter
	funcs       map[string]interface{} // 注册的自定义函数
}

// New 创建新的 SQL 模板引擎
func New() *Engine {
	return &Engine{
		store:       NewTemplateStore(),
		compiledAST: make(map[string]*TemplateAST),
		interp:      interpreter.New(),
		funcs:       make(map[string]interface{}),
	}
}

// RegisterFunc 注册自定义函数
func (e *Engine) RegisterFunc(name string, fn interface{}) {
	e.funcs[name] = fn
}

// LoadMarkdown 加载 markdown 文件内容
func (e *Engine) LoadMarkdown(content string) error {
	if err := e.store.LoadMarkdown(content); err != nil {
		return err
	}

	// 预编译所有模板
	for key, tmpl := range e.store.templates {
		ast, err := ParseTemplate(tmpl.Content)
		if err != nil {
			return fmt.Errorf("template %s: %w", key, err)
		}
		ast.Namespace = tmpl.Namespace
		ast.Name = tmpl.Name
		e.compiledAST[key] = ast
	}

	return nil
}

// GetSql 获取渲染后的 SQL 和参数
// path: 模板路径，格式为 "namespace.name" 或 "namespace.name.define"
// args: 模板渲染的 scope（任意类型，会被展开为变量）
func (e *Engine) GetSql(path string, args interface{}) (Query, error) {
	// 解析路径
	parts := strings.Split(path, ".")
	if len(parts) < 2 {
		return Query{}, fmt.Errorf("invalid path: %s, expected format: namespace.name", path)
	}

	namespace := parts[0]
	name := parts[1]
	defineName := ""
	if len(parts) > 2 {
		defineName = parts[2]
	}

	key := namespace + "." + name

	// 获取 AST
	ast, ok := e.compiledAST[key]
	if !ok {
		return Query{}, fmt.Errorf("template not found: %s", key)
	}

	// 创建执行上下文
	ctx := newExecutionContext(e, args)

	// 如果指定了 define 名称，只执行该 define 块
	if defineName != "" {
		defineNode := findDefine(ast.Nodes, defineName)
		if defineNode == nil {
			return Query{}, fmt.Errorf("define not found: %s in template %s", defineName, key)
		}
		if err := ctx.executeNodes(defineNode.Body); err != nil {
			return Query{}, err
		}
	} else {
		// 执行整个模板
		if err := ctx.executeNodes(ast.Nodes); err != nil {
			return Query{}, err
		}
	}

	return Query{
		SQL:    ctx.sql.String(),
		Params: ctx.args,
	}, nil
}

// findDefine 在节点列表中查找 define 块
func findDefine(nodes []Node, name string) *DefineNode {
	for _, node := range nodes {
		switch n := node.(type) {
		case *DefineNode:
			if n.Name == name {
				return n
			}
			// 递归查找嵌套的 define
			if found := findDefine(n.Body, name); found != nil {
				return found
			}
		case *IfNode:
			if found := findDefine(n.Body, name); found != nil {
				return found
			}
			for _, ei := range n.ElseIf {
				if found := findDefine(ei.Body, name); found != nil {
					return found
				}
			}
			if n.Else != nil {
				if found := findDefine(n.Else.Body, name); found != nil {
					return found
				}
			}
		case *ForNode:
			if found := findDefine(n.Body, name); found != nil {
				return found
			}
		}
	}
	return nil
}

// executionContext 执行上下文
type executionContext struct {
	engine     *Engine
	scope      map[string]interface{}
	sql        strings.Builder
	args       []interface{}
	covers     map[string][]Node // cover 覆盖
	interp     *interpreter.Interpreter
	scopeObj   interface{}     // 原始 scope 对象（用于方法调用）
	typeInfo   *CachedTypeInfo // 缓存的类型信息
	lineBuffer strings.Builder // 行缓冲（用于条件行控制）
	lineArgs   []interface{}   // 行参数缓冲
	inCondLine bool            // 是否在条件行中
	condResult bool            // 条件结果
}

// newExecutionContext 创建执行上下文
func newExecutionContext(engine *Engine, args interface{}) *executionContext {
	ctx := &executionContext{
		engine:   engine,
		scope:    getScope(),
		covers:   make(map[string][]Node),
		interp:   interpreter.New(),
		scopeObj: args,
	}

	// 绑定引擎注册的函数
	for name, fn := range engine.funcs {
		ctx.scope[name] = fn
		ctx.interp.BindFunc(name, fn)
	}

	// 将 args 展开到 scope（使用缓存的类型信息）
	if args != nil {
		ctx.expandToScopeWithCache(args)
	}

	return ctx
}

// expandToScopeWithCache 使用缓存将值展开到 scope
func (ctx *executionContext) expandToScopeWithCache(args interface{}) {
	rv := reflect.ValueOf(args)
	rt := rv.Type()

	// 获取缓存的类型信息
	ctx.typeInfo = GetTypeInfo(rt)

	// 如果是指针，获取指向的值
	if rv.Kind() == reflect.Ptr {
		if rv.IsNil() {
			return
		}
		rv = rv.Elem()
	}

	switch rv.Kind() {
	case reflect.Struct:
		// 使用缓存的字段信息
		ctx.expandStructFields(rv)
		// 绑定方法（使用缓存）
		ctx.bindMethodsWithCache(reflect.ValueOf(args))

	case reflect.Map:
		// map：遍历键值对
		for _, key := range rv.MapKeys() {
			if key.Kind() == reflect.String {
				ctx.scope[key.String()] = rv.MapIndex(key).Interface()
			}
		}
	}
}

// expandStructFields 使用缓存展开结构体字段（包括嵌入字段和私有字段）
func (ctx *executionContext) expandStructFields(rv reflect.Value) {
	rt := rv.Type()

	for i := 0; i < rv.NumField(); i++ {
		field := rt.Field(i)
		fieldValue := rv.Field(i)

		// 对于私有字段，使用 unsafe 获取值
		if !field.IsExported() {
			// 使用 unsafe 获取私有字段值
			fieldValue = getUnexportedField(rv, i)
		}

		if field.Anonymous {
			// 嵌入字段，递归展开
			embeddedValue := fieldValue
			if embeddedValue.Kind() == reflect.Ptr {
				if !embeddedValue.IsValid() || embeddedValue.IsNil() {
					continue
				}
				embeddedValue = embeddedValue.Elem()
			}
			if embeddedValue.IsValid() && embeddedValue.Kind() == reflect.Struct {
				ctx.expandStructFields(embeddedValue)
				// 也绑定嵌入结构体的方法
				if fieldValue.IsValid() {
					ctx.bindMethodsWithCache(fieldValue)
				}
			}
		}

		if !fieldValue.IsValid() {
			continue
		}

		// 添加字段值
		lowerName := toLowerFirst(field.Name)
		if fieldValue.CanInterface() {
			ctx.scope[lowerName] = fieldValue.Interface()
			ctx.scope[field.Name] = fieldValue.Interface()
		} else {
			// 私有字段，使用 unsafe 获取
			val := getUnexportedFieldValue(fieldValue)
			ctx.scope[lowerName] = val
			ctx.scope[field.Name] = val
		}
	}
}

// getUnexportedField 获取未导出的字段值
func getUnexportedField(rv reflect.Value, index int) reflect.Value {
	field := rv.Field(index)
	if field.CanInterface() {
		return field
	}
	// 使用 unsafe 获取私有字段
	if field.CanAddr() {
		return reflect.NewAt(field.Type(), unsafe.Pointer(field.UnsafeAddr())).Elem()
	}
	// 无法获取地址的情况，返回无效值
	return reflect.Value{}
}

// getUnexportedFieldValue 获取未导出字段的值
func getUnexportedFieldValue(field reflect.Value) interface{} {
	if !field.IsValid() {
		return nil
	}
	if field.CanInterface() {
		return field.Interface()
	}
	// 使用 unsafe 获取值
	if field.CanAddr() {
		return reflect.NewAt(field.Type(), unsafe.Pointer(field.UnsafeAddr())).Elem().Interface()
	}
	// 无法获取地址的情况
	return nil
}

// bindMethodsWithCache 使用缓存绑定方法
func (ctx *executionContext) bindMethodsWithCache(rv reflect.Value) {
	if !rv.IsValid() {
		return
	}

	rt := rv.Type()
	typeInfo := GetTypeInfo(rt)

	// 绑定值接收器方法
	for name, methodInfo := range typeInfo.Methods {
		if _, exists := ctx.scope[name]; !exists {
			ctx.scope[name] = rv.Method(methodInfo.Index).Interface()
			ctx.interp.BindFunc(name, rv.Method(methodInfo.Index).Interface())
		}
	}

	// 绑定指针接收器方法
	if rv.Kind() != reflect.Ptr {
		// 创建可寻址的副本
		ptrRv := reflect.New(rv.Type())
		ptrRv.Elem().Set(rv)
		for name, methodInfo := range typeInfo.PtrMethods {
			if _, exists := ctx.scope[name]; !exists {
				ctx.scope[name] = ptrRv.Method(methodInfo.Index).Interface()
				ctx.interp.BindFunc(name, ptrRv.Method(methodInfo.Index).Interface())
			}
		}
	} else {
		// rv 已经是指针
		ptrTypeInfo := GetTypeInfo(rv.Type())
		for name, methodInfo := range ptrTypeInfo.Methods {
			if _, exists := ctx.scope[name]; !exists {
				ctx.scope[name] = rv.Method(methodInfo.Index).Interface()
				ctx.interp.BindFunc(name, rv.Method(methodInfo.Index).Interface())
			}
		}
	}
}

// executeNodes 执行节点列表
func (ctx *executionContext) executeNodes(nodes []Node) error {
	for _, node := range nodes {
		if err := ctx.executeNode(node); err != nil {
			return err
		}
	}
	return nil
}

// executeNode 执行单个节点
func (ctx *executionContext) executeNode(node Node) error {
	switch n := node.(type) {
	case *TextNode:
		ctx.sql.WriteString(n.Text)
		return nil

	case *VarNode:
		return ctx.executeVarNode(n)

	case *VarExprNode:
		return ctx.executeVarExprNode(n)

	case *RawNode:
		return ctx.executeRawNode(n)

	case *RawExprNode:
		return ctx.executeRawExprNode(n)

	case *IfNode:
		return ctx.executeIf(n)

	case *ForNode:
		return ctx.executeFor(n)

	case *CodeNode:
		return ctx.executeCode(n.Code)

	case *UseNode:
		return ctx.executeUse(n)

	case *DefineNode:
		return ctx.executeDefine(n)

	case *ConditionalLineNode:
		return ctx.executeConditionalLine(n)

	case *FuncBlockNode:
		return ctx.executeFuncBlock(n)

	default:
		return fmt.Errorf("unknown node type: %T", node)
	}
}

// executeVarNode 执行变量节点
func (ctx *executionContext) executeVarNode(n *VarNode) error {
	value, ok := ctx.scope[n.Name]

	if n.Conditional {
		// 条件控制：如果字段不存在或值为假，跳过当前行
		if !ok || !ctx.isTruthy(value) {
			ctx.skipCurrentLine()
			return nil
		}
	} else if !ok {
		return fmt.Errorf("variable not found: %s", n.Name)
	}

	ctx.appendArg(value)
	return nil
}

// executeVarExprNode 执行变量表达式节点
func (ctx *executionContext) executeVarExprNode(n *VarExprNode) error {
	value, err := ctx.evalExpr(n.Expr)
	if err != nil {
		return err
	}

	if n.Conditional {
		// 条件控制：检查值是否为 "真"
		if !ctx.isTruthy(value) {
			ctx.skipCurrentLine()
			return nil
		}
	}

	ctx.appendArg(value)
	return nil
}

// executeRawNode 执行直接输出变量节点
func (ctx *executionContext) executeRawNode(n *RawNode) error {
	value, ok := ctx.scope[n.Name]

	if n.Conditional {
		// 条件控制：如果字段不存在或值为假，跳过当前行
		if !ok || !ctx.isTruthy(value) {
			ctx.skipCurrentLine()
			return nil
		}
	} else if !ok {
		return fmt.Errorf("variable not found: %s", n.Name)
	}

	ctx.sql.WriteString(fmt.Sprintf("%v", value))
	return nil
}

// executeRawExprNode 执行直接输出表达式节点
func (ctx *executionContext) executeRawExprNode(n *RawExprNode) error {
	value, err := ctx.evalExpr(n.Expr)
	if err != nil {
		return err
	}

	if n.Conditional {
		if !ctx.isTruthy(value) {
			ctx.skipCurrentLine()
			return nil
		}
	}

	ctx.sql.WriteString(fmt.Sprintf("%v", value))
	return nil
}

// executeConditionalLine 执行条件行节点
func (ctx *executionContext) executeConditionalLine(n *ConditionalLineNode) error {
	// 评估条件
	result, err := ctx.evalCondition(n.Condition)
	if err != nil {
		return err
	}

	if !result {
		return nil // 条件为假，跳过整行
	}

	// 条件为真，执行行内所有节点
	return ctx.executeNodes(n.LineNodes)
}

// isTruthy 判断值是否为 "真"
func (ctx *executionContext) isTruthy(value interface{}) bool {
	if value == nil {
		return false
	}

	rv := reflect.ValueOf(value)
	switch rv.Kind() {
	case reflect.Bool:
		return rv.Bool()
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return rv.Int() != 0
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return rv.Uint() != 0
	case reflect.Float32, reflect.Float64:
		return rv.Float() != 0
	case reflect.String:
		return rv.String() != ""
	case reflect.Slice, reflect.Array, reflect.Map:
		return rv.Len() > 0
	case reflect.Ptr, reflect.Interface:
		return !rv.IsNil()
	default:
		return true
	}
}

// skipCurrentLine 跳过当前行（移除到上一个换行符之后的内容）
func (ctx *executionContext) skipCurrentLine() {
	sql := ctx.sql.String()
	lastNewline := strings.LastIndex(sql, "\n")
	if lastNewline >= 0 {
		ctx.sql.Reset()
		ctx.sql.WriteString(sql[:lastNewline+1])
	} else {
		ctx.sql.Reset()
	}
}

// executeFuncBlock 执行函数块节点 @ func() {}
func (ctx *executionContext) executeFuncBlock(n *FuncBlockNode) error {
	// 先执行块内节点，生成 Query
	subCtx := &executionContext{
		engine:   ctx.engine,
		scope:    ctx.scope,
		covers:   ctx.covers,
		interp:   ctx.interp,
		scopeObj: ctx.scopeObj,
		typeInfo: ctx.typeInfo,
	}

	if err := subCtx.executeNodes(n.Body); err != nil {
		return err
	}

	// 创建 Query 对象（优先以指针形式传递，便于函数块直接修改 SQL/Params）
	query := &Query{
		SQL:    subCtx.sql.String(),
		Params: subCtx.args,
	}

	// 调用函数，传入 Query 作为最后一个参数
	// 构造调用表达式
	funcExpr := strings.TrimSpace(n.FuncExpr)

	// 检查是否是 scope 中的函数
	if fn, ok := ctx.scope[funcExpr]; ok {
		// 直接调用无参函数
		if fnVal := reflect.ValueOf(fn); fnVal.Kind() == reflect.Func {
			fnType := fnVal.Type()
			// 优先支持 func(*Query)
			if fnType.NumIn() == 1 && fnType.In(0) == reflect.TypeOf(&Query{}) {
				results := fnVal.Call([]reflect.Value{reflect.ValueOf(query)})
				if len(results) > 0 {
					result := results[0].Interface()
					if s, ok := result.(string); ok {
						query.SQL = s
					} else if q, ok := result.(Query); ok {
						*query = q
					} else if qp, ok := result.(*Query); ok && qp != nil {
						query = qp
					}
				}
				ctx.sql.WriteString(query.SQL)
				ctx.args = append(ctx.args, query.Params...)
				return nil
			}
			// 兼容旧：func(Query)
			if fnType.NumIn() == 1 && fnType.In(0) == reflect.TypeOf(Query{}) {
				results := fnVal.Call([]reflect.Value{reflect.ValueOf(*query)})
				if len(results) > 0 {
					result := results[0].Interface()
					if s, ok := result.(string); ok {
						query.SQL = s
					} else if q, ok := result.(Query); ok {
						*query = q
					}
				}
				ctx.sql.WriteString(query.SQL)
				ctx.args = append(ctx.args, query.Params...)
				return nil
			}
		}
	}

	// 尝试解析并调用函数表达式
	// 如果表达式包含括号，需要注入 Query 参数
	if strings.Contains(funcExpr, "(") {
		// 替换最后的 ) 为 , query)
		lastParen := strings.LastIndex(funcExpr, ")")
		if lastParen > 0 {
			// 检查是否是空括号
			openParen := strings.LastIndex(funcExpr[:lastParen], "(")
			if openParen >= 0 {
				between := strings.TrimSpace(funcExpr[openParen+1 : lastParen])
				if between == "" {
					// 空括号，直接传 query
					funcExpr = funcExpr[:openParen+1] + "__query__" + funcExpr[lastParen:]
				} else {
					// 有参数，追加 query
					funcExpr = funcExpr[:lastParen] + ", __query__" + funcExpr[lastParen:]
				}
			}
		}
	} else {
		// 没有括号，添加 (query)
		funcExpr = funcExpr + "(__query__)"
	}

	// 绑定 query 到作用域（指针），便于函数直接修改
	ctx.scope["__query__"] = query
	ctx.interp.BindValue("__query__", query)

	// 调用函数
	result, err := ctx.evalExpr(funcExpr)
	if err != nil {
		// 如果函数调用失败，直接输出块内容
		ctx.sql.WriteString(subCtx.sql.String())
		ctx.args = append(ctx.args, subCtx.args...)
		return nil
	}

	// 如果函数返回值存在，则作为兼容逻辑；推荐直接修改 *Query
	if result != nil {
		if s, ok := result.(string); ok {
			query.SQL = s
		} else if q, ok := result.(Query); ok {
			*query = q
		} else if qp, ok := result.(*Query); ok && qp != nil {
			query = qp
		}
	}

	ctx.sql.WriteString(query.SQL)
	ctx.args = append(ctx.args, query.Params...)

	return nil
}

// executeIf 执行 if 节点
func (ctx *executionContext) executeIf(n *IfNode) error {
	// 评估条件
	result, err := ctx.evalCondition(n.Condition)
	if err != nil {
		return err
	}

	if result {
		return ctx.executeNodes(n.Body)
	}

	// 检查 else if
	for _, elseIf := range n.ElseIf {
		result, err := ctx.evalCondition(elseIf.Condition)
		if err != nil {
			return err
		}
		if result {
			return ctx.executeNodes(elseIf.Body)
		}
	}

	// else
	if n.Else != nil {
		return ctx.executeNodes(n.Else.Body)
	}

	return nil
}

// executeFor 执行 for 节点
func (ctx *executionContext) executeFor(n *ForNode) error {
	expr := strings.TrimSpace(n.Expr)

	// 判断是 range 形式还是传统 for 形式
	if strings.Contains(expr, "range") {
		return ctx.executeForRange(n)
	}
	return ctx.executeForTraditional(n)
}

// executeForRange 执行 range 形式的 for 循环
func (ctx *executionContext) executeForRange(n *ForNode) error {
	expr := strings.TrimSpace(n.Expr)

	// 解析 range 表达式：i, v := range xxx
	parts := strings.SplitN(expr, ":=", 2)
	if len(parts) != 2 {
		return fmt.Errorf("invalid range expression: %s", expr)
	}

	varPart := strings.TrimSpace(parts[0])
	rangePart := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(parts[1]), "range"))

	// 解析变量名
	varNames := strings.Split(varPart, ",")
	indexVar := ""
	valueVar := ""
	if len(varNames) >= 1 {
		indexVar = strings.TrimSpace(varNames[0])
	}
	if len(varNames) >= 2 {
		valueVar = strings.TrimSpace(varNames[1])
	}

	// 评估 range 表达式
	rangeValue, err := ctx.evalExpr(rangePart)
	if err != nil {
		return fmt.Errorf("range expression error: %w", err)
	}

	rv := reflect.ValueOf(rangeValue)
	switch rv.Kind() {
	case reflect.Slice, reflect.Array:
		for i := 0; i < rv.Len(); i++ {
			// 设置循环变量
			if indexVar != "" && indexVar != "_" {
				ctx.scope[indexVar] = i
			}
			if valueVar != "" && valueVar != "_" {
				ctx.scope[valueVar] = rv.Index(i).Interface()
			}

			// 执行 body
			if err := ctx.executeNodes(n.Body); err != nil {
				return err
			}
		}
	case reflect.Map:
		for _, key := range rv.MapKeys() {
			if indexVar != "" && indexVar != "_" {
				ctx.scope[indexVar] = key.Interface()
			}
			if valueVar != "" && valueVar != "_" {
				ctx.scope[valueVar] = rv.MapIndex(key).Interface()
			}

			if err := ctx.executeNodes(n.Body); err != nil {
				return err
			}
		}
	default:
		return fmt.Errorf("cannot range over %s", rv.Kind())
	}

	return nil
}

// executeForTraditional 执行传统 for 循环
func (ctx *executionContext) executeForTraditional(n *ForNode) error {
	expr := strings.TrimSpace(n.Expr)

	// 解析 for i := 0; i < 10; i++
	parts := strings.Split(expr, ";")
	if len(parts) != 3 {
		return fmt.Errorf("invalid for expression: %s", expr)
	}

	initPart := strings.TrimSpace(parts[0])
	condPart := strings.TrimSpace(parts[1])
	postPart := strings.TrimSpace(parts[2])

	// 执行初始化
	if strings.Contains(initPart, ":=") {
		initParts := strings.SplitN(initPart, ":=", 2)
		varName := strings.TrimSpace(initParts[0])
		initValue, err := ctx.evalExpr(strings.TrimSpace(initParts[1]))
		if err != nil {
			return fmt.Errorf("for init error: %w", err)
		}
		ctx.scope[varName] = initValue

		// 循环
		for {
			// 检查条件
			cond, err := ctx.evalCondition(condPart)
			if err != nil {
				return fmt.Errorf("for condition error: %w", err)
			}
			if !cond {
				break
			}

			// 执行 body
			if err := ctx.executeNodes(n.Body); err != nil {
				return err
			}

			// 执行 post
			if err := ctx.executePost(varName, postPart); err != nil {
				return err
			}
		}
	}

	return nil
}

// executePost 执行 for 循环的 post 语句
func (ctx *executionContext) executePost(varName, postPart string) error {
	postPart = strings.TrimSpace(postPart)

	// 处理 i++
	if strings.HasSuffix(postPart, "++") {
		vName := strings.TrimSuffix(postPart, "++")
		vName = strings.TrimSpace(vName)
		if v, ok := ctx.scope[vName]; ok {
			if iv, ok := v.(int); ok {
				ctx.scope[vName] = iv + 1
			}
		}
		return nil
	}

	// 处理 i--
	if strings.HasSuffix(postPart, "--") {
		vName := strings.TrimSuffix(postPart, "--")
		vName = strings.TrimSpace(vName)
		if v, ok := ctx.scope[vName]; ok {
			if iv, ok := v.(int); ok {
				ctx.scope[vName] = iv - 1
			}
		}
		return nil
	}

	// 处理 i += n
	if strings.Contains(postPart, "+=") {
		parts := strings.SplitN(postPart, "+=", 2)
		vName := strings.TrimSpace(parts[0])
		addValue, err := ctx.evalExpr(strings.TrimSpace(parts[1]))
		if err != nil {
			return err
		}
		if v, ok := ctx.scope[vName]; ok {
			if iv, ok := v.(int); ok {
				if av, ok := addValue.(int); ok {
					ctx.scope[vName] = iv + av
				}
			}
		}
		return nil
	}

	return nil
}

// executeCode 执行直接代码
func (ctx *executionContext) executeCode(code string) error {
	interp := interpreter.New()
	for name, value := range ctx.scope {
		interp.BindValue(name, value)
	}

	// 直接执行代码（需要包装成合法的 Go 代码）
	wrappedCode := fmt.Sprintf(`
package main

func main() {
	%s
}
`, code)

	_, err := interp.Eval(wrappedCode)
	return err
}

// executeUse 执行 use 节点
func (ctx *executionContext) executeUse(n *UseNode) error {
	// 解析路径
	parts := strings.Split(n.Path, ".")
	if len(parts) < 2 {
		return fmt.Errorf("invalid use path: %s", n.Path)
	}

	namespace := parts[0]
	name := parts[1]
	defineName := ""
	if len(parts) > 2 {
		defineName = parts[2]
	}

	key := namespace + "." + name

	// 获取目标模板的 AST
	ast, ok := ctx.engine.compiledAST[key]
	if !ok {
		return fmt.Errorf("template not found: %s", key)
	}

	// 设置 covers
	oldCovers := ctx.covers
	ctx.covers = make(map[string][]Node)
	for _, cover := range n.Covers {
		ctx.covers[cover.Name] = cover.Body
	}

	// 如果指定了 define，只执行该 define
	if defineName != "" {
		defineNode := findDefine(ast.Nodes, defineName)
		if defineNode == nil {
			return fmt.Errorf("define not found: %s in template %s", defineName, key)
		}
		if err := ctx.executeNodes(defineNode.Body); err != nil {
			return err
		}
	} else {
		// 执行整个模板
		if err := ctx.executeNodes(ast.Nodes); err != nil {
			return err
		}
	}

	// 恢复 covers
	ctx.covers = oldCovers

	return nil
}

// executeDefine 执行 define 节点
func (ctx *executionContext) executeDefine(n *DefineNode) error {
	// 检查是否有 cover 覆盖
	if coverBody, ok := ctx.covers[n.Name]; ok {
		return ctx.executeNodes(coverBody)
	}

	// 没有覆盖，执行原始内容
	return ctx.executeNodes(n.Body)
}

// appendArg 添加参数（支持数组展开）
func (ctx *executionContext) appendArg(value interface{}) {
	rv := reflect.ValueOf(value)

	if rv.Kind() == reflect.Slice || rv.Kind() == reflect.Array {
		n := rv.Len()
		for i := 0; i < n; i++ {
			if i > 0 {
				ctx.sql.WriteString(", ")
			}
			ctx.sql.WriteString("?")
			ctx.args = append(ctx.args, rv.Index(i).Interface())
		}
	} else {
		ctx.sql.WriteString("?")
		ctx.args = append(ctx.args, value)
	}
}

// evalExpr 评估表达式
func (ctx *executionContext) evalExpr(expr string) (interface{}, error) {
	// 使用 goscript2 评估表达式
	return ctx.interp.EvalExprWithArgs(expr, ctx.scope)
}

// evalCondition 评估条件表达式
func (ctx *executionContext) evalCondition(condition string) (bool, error) {
	result, err := ctx.evalExpr(condition)
	if err != nil {
		return false, fmt.Errorf("condition error: %w", err)
	}

	// 转换为 bool
	switch v := result.(type) {
	case bool:
		return v, nil
	case int, int8, int16, int32, int64:
		return reflect.ValueOf(v).Int() != 0, nil
	case uint, uint8, uint16, uint32, uint64:
		return reflect.ValueOf(v).Uint() != 0, nil
	case float32, float64:
		return reflect.ValueOf(v).Float() != 0, nil
	case string:
		return v != "", nil
	case nil:
		return false, nil
	default:
		return !reflect.ValueOf(v).IsZero(), nil
	}
}

// GetSql 全局便捷函数
var defaultEngine *Engine

// Init 初始化默认引擎
func Init() *Engine {
	defaultEngine = New()
	return defaultEngine
}

// Load 加载 markdown 到默认引擎
func Load(content string) error {
	if defaultEngine == nil {
		Init()
	}
	return defaultEngine.LoadMarkdown(content)
}

// GetSqlFromDefault 从默认引擎获取 SQL
func GetSqlFromDefault(path string, args interface{}) (Query, error) {
	if defaultEngine == nil {
		return Query{}, fmt.Errorf("engine not initialized, call Init() first")
	}
	return defaultEngine.GetSql(path, args)
}

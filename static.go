package gosql

import (
	"fmt"
	"reflect"
	"strings"
	"sync"

	"github.com/llyb120/goscript2/interpreter"
)

// StaticFunc 静态模板执行函数类型
type StaticFunc func(ctx *StaticContext) error

// 全局静态函数注册表
var (
	staticMu    sync.RWMutex
	staticFuncs = make(map[string]StaticFunc)
)

// RegisterStatic 注册静态模板函数（通常在 init() 中由生成的代码调用）
func RegisterStatic(path string, fn StaticFunc) {
	staticMu.Lock()
	defer staticMu.Unlock()
	staticFuncs[path] = fn
}

// getStaticFunc 获取静态函数
func getStaticFunc(path string) (StaticFunc, bool) {
	staticMu.RLock()
	defer staticMu.RUnlock()
	fn, ok := staticFuncs[path]
	return fn, ok
}

// StaticContext 静态模式执行上下文
// 注意：不包含解释器，所有表达式在代码生成阶段已编译为原生 Go 代码
type StaticContext struct {
	engine     *Engine
	sql        strings.Builder
	args       []interface{}
	scope      map[string]interface{}
	scopeObj   interface{}
	typeInfo   *CachedTypeInfo
	covers     map[string]func(*StaticContext) error
	definePath []string
}

// NewStaticContext 创建静态执行上下文
func NewStaticContext(engine *Engine, args interface{}) *StaticContext {
	ctx := &StaticContext{
		engine:   engine,
		scope:    make(map[string]interface{}, 16),
		scopeObj: args,
		covers:   make(map[string]func(*StaticContext) error),
	}

	// 绑定引擎注册的函数
	for name, fn := range engine.funcs {
		ctx.scope[name] = fn
	}

	// 展开 args 到 scope
	if args != nil {
		ctx.expandScope(args)
	}

	return ctx
}

// expandScope 展开 scope
func (ctx *StaticContext) expandScope(args interface{}) {
	rv := reflect.ValueOf(args)
	rt := rv.Type()
	ctx.typeInfo = GetTypeInfo(rt)

	if rv.Kind() == reflect.Ptr {
		if rv.IsNil() {
			return
		}
		rv = rv.Elem()
	}

	switch rv.Kind() {
	case reflect.Struct:
		ctx.expandStructFields(rv)
		ctx.bindMethods(reflect.ValueOf(args))
	case reflect.Map:
		for _, key := range rv.MapKeys() {
			if key.Kind() == reflect.String {
				ctx.scope[key.String()] = rv.MapIndex(key).Interface()
			}
		}
	}
}

// expandStructFields 展开结构体字段
func (ctx *StaticContext) expandStructFields(rv reflect.Value) {
	rt := rv.Type()
	for i := 0; i < rv.NumField(); i++ {
		field := rt.Field(i)
		fieldValue := rv.Field(i)

		if !field.IsExported() {
			fieldValue = getUnexportedField(rv, i)
		}

		if field.Anonymous {
			embeddedValue := fieldValue
			if embeddedValue.Kind() == reflect.Ptr {
				if !embeddedValue.IsValid() || embeddedValue.IsNil() {
					continue
				}
				embeddedValue = embeddedValue.Elem()
			}
			if embeddedValue.IsValid() && embeddedValue.Kind() == reflect.Struct {
				ctx.expandStructFields(embeddedValue)
				if fieldValue.IsValid() {
					ctx.bindMethods(fieldValue)
				}
			}
		}

		if !fieldValue.IsValid() {
			continue
		}

		lowerName := toLowerFirst(field.Name)
		if fieldValue.CanInterface() {
			ctx.scope[lowerName] = fieldValue.Interface()
			ctx.scope[field.Name] = fieldValue.Interface()
		} else {
			val := getUnexportedFieldValue(fieldValue)
			ctx.scope[lowerName] = val
			ctx.scope[field.Name] = val
		}
	}
}

// bindMethods 绑定方法（不使用解释器）
func (ctx *StaticContext) bindMethods(rv reflect.Value) {
	if !rv.IsValid() {
		return
	}

	rt := rv.Type()
	typeInfo := GetTypeInfo(rt)

	for name, methodInfo := range typeInfo.Methods {
		if _, exists := ctx.scope[name]; !exists {
			ctx.scope[name] = rv.Method(methodInfo.Index).Interface()
		}
	}

	if rv.Kind() != reflect.Ptr {
		ptrRv := reflect.New(rv.Type())
		ptrRv.Elem().Set(rv)
		for name, methodInfo := range typeInfo.PtrMethods {
			if _, exists := ctx.scope[name]; !exists {
				ctx.scope[name] = ptrRv.Method(methodInfo.Index).Interface()
			}
		}
	} else {
		ptrTypeInfo := GetTypeInfo(rv.Type())
		for name, methodInfo := range ptrTypeInfo.Methods {
			if _, exists := ctx.scope[name]; !exists {
				ctx.scope[name] = rv.Method(methodInfo.Index).Interface()
			}
		}
	}
}

// ==================== 基础方法（供生成的静态代码使用）====================

// Recover 恢复 panic（在生成代码的 defer 中使用）
func (ctx *StaticContext) Recover(retErr *error) {
	if r := recover(); r != nil {
		if err, ok := r.(error); ok {
			*retErr = err
		} else {
			*retErr = fmt.Errorf("gosql static: %v", r)
		}
	}
}

// Get 获取变量值
func (ctx *StaticContext) Get(name string) (interface{}, bool) {
	v, ok := ctx.scope[name]
	return v, ok
}

// MustGet 获取变量值，不存在时 panic
func (ctx *StaticContext) MustGet(name string) interface{} {
	v, ok := ctx.scope[name]
	if !ok {
		panic(fmt.Errorf("variable not found: %s", name))
	}
	return v
}

// Set 设置变量
func (ctx *StaticContext) Set(name string, value interface{}) {
	ctx.scope[name] = value
}

// WriteSQL 写入 SQL 文本
func (ctx *StaticContext) WriteSQL(s string) {
	ctx.sql.WriteString(s)
}

// AppendArg 添加参数（支持数组展开为多个 ? ）
func (ctx *StaticContext) AppendArg(value interface{}) {
	rv := reflect.ValueOf(value)
	if rv.Kind() == reflect.Slice || rv.Kind() == reflect.Array {
		for i := 0; i < rv.Len(); i++ {
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

// AppendRaw 直接输出值到 SQL
func (ctx *StaticContext) AppendRaw(value interface{}) {
	ctx.sql.WriteString(fmt.Sprintf("%v", value))
}

// IsTruthy 判断值是否为"真"
func (ctx *StaticContext) IsTruthy(value interface{}) bool {
	return ToBool(value)
}

// SkipCurrentLine 跳过当前行
func (ctx *StaticContext) SkipCurrentLine() {
	sql := ctx.sql.String()
	lastNewline := strings.LastIndex(sql, "\n")
	if lastNewline >= 0 {
		ctx.sql.Reset()
		ctx.sql.WriteString(sql[:lastNewline+1])
	} else {
		ctx.sql.Reset()
	}
}

// Build 构建查询结果
func (ctx *StaticContext) Build() Query {
	return Query{
		SQL:    ctx.sql.String(),
		Params: ctx.args,
	}
}

// BuildPtr 构建查询结果（返回指针）
func (ctx *StaticContext) BuildPtr() *Query {
	return &Query{
		SQL:    ctx.sql.String(),
		Params: ctx.args,
	}
}

// WriteQuery 将 Query 的 SQL 和参数写入当前上下文
func (ctx *StaticContext) WriteQuery(q *Query) {
	ctx.sql.WriteString(q.SQL)
	ctx.args = append(ctx.args, q.Params...)
}

// ==================== 函数调用（纯反射，无解释器）====================

// Call 调用 scope 中的函数
// 生成代码示例: ctx.Call("GetName") 或 ctx.Call("trim", "and")
func (ctx *StaticContext) Call(name string, args ...interface{}) interface{} {
	fn, ok := ctx.scope[name]
	if !ok {
		panic(fmt.Errorf("function not found: %s", name))
	}
	fnVal := reflect.ValueOf(fn)
	if fnVal.Kind() != reflect.Func {
		panic(fmt.Errorf("%s is not a function", name))
	}

	fnType := fnVal.Type()
	in := make([]reflect.Value, len(args))
	for i, arg := range args {
		if arg == nil {
			if i < fnType.NumIn() {
				in[i] = reflect.Zero(fnType.In(i))
			} else {
				in[i] = reflect.ValueOf((*interface{})(nil))
			}
		} else {
			av := reflect.ValueOf(arg)
			// 参数类型转换
			if i < fnType.NumIn() {
				expected := fnType.In(i)
				if av.Type() != expected && av.Type().ConvertibleTo(expected) {
					av = av.Convert(expected)
				}
			}
			in[i] = av
		}
	}

	results := fnVal.Call(in)
	if len(results) > 0 {
		return results[0].Interface()
	}
	return nil
}

// CallMethod 调用 scope 中对象的方法
// 生成代码示例: ctx.CallMethod("obj", "Method", arg1, arg2)
func (ctx *StaticContext) CallMethod(objName, methodName string, args ...interface{}) interface{} {
	obj, ok := ctx.scope[objName]
	if !ok {
		panic(fmt.Errorf("object not found: %s", objName))
	}
	rv := reflect.ValueOf(obj)
	method := rv.MethodByName(methodName)
	if !method.IsValid() {
		// 尝试指针
		if rv.Kind() != reflect.Ptr {
			ptrRv := reflect.New(rv.Type())
			ptrRv.Elem().Set(rv)
			method = ptrRv.MethodByName(methodName)
		}
	}
	if !method.IsValid() {
		panic(fmt.Errorf("method not found: %s.%s", objName, methodName))
	}

	in := make([]reflect.Value, len(args))
	for i, arg := range args {
		if arg == nil {
			in[i] = reflect.Zero(method.Type().In(i))
		} else {
			in[i] = reflect.ValueOf(arg)
		}
	}

	results := method.Call(in)
	if len(results) > 0 {
		return results[0].Interface()
	}
	return nil
}

// ==================== SubContext / FuncBlock ====================

// SubContext 创建子上下文（用于 FuncBlock 体执行）
func (ctx *StaticContext) SubContext() *StaticContext {
	return &StaticContext{
		engine:   ctx.engine,
		scope:    ctx.scope,
		scopeObj: ctx.scopeObj,
		typeInfo: ctx.typeInfo,
		covers:   ctx.covers,
	}
}

// CallFuncBlock 调用函数块
// funcName: 函数名（如 "trim"）
// query: 子上下文构建的查询（指针，会被函数修改）
// args: 函数的额外参数（如 "and"）
func (ctx *StaticContext) CallFuncBlock(funcName string, query *Query, args ...interface{}) {
	fn, ok := ctx.scope[funcName]
	if !ok {
		return // 函数不存在，静默跳过
	}
	fnVal := reflect.ValueOf(fn)
	if fnVal.Kind() != reflect.Func {
		return
	}

	fnType := fnVal.Type()
	numIn := fnType.NumIn()

	// 策略 1: func(*Query) 或 func(Query)
	if numIn == 1 && len(args) == 0 {
		var results []reflect.Value
		if fnType.In(0) == reflect.TypeOf(&Query{}) {
			results = fnVal.Call([]reflect.Value{reflect.ValueOf(query)})
		} else if fnType.In(0) == reflect.TypeOf(Query{}) {
			results = fnVal.Call([]reflect.Value{reflect.ValueOf(*query)})
		} else {
			return
		}
		if len(results) > 0 {
			ctx.handleFuncResult(results[0].Interface(), query)
		}
		return
	}

	// 策略 2: func(args..., *Query) 或 func(args..., Query)
	if numIn == len(args)+1 {
		callArgs := make([]reflect.Value, 0, numIn)
		for i, a := range args {
			av := reflect.ValueOf(a)
			expected := fnType.In(i)
			if av.Type() != expected && av.Type().ConvertibleTo(expected) {
				av = av.Convert(expected)
			}
			callArgs = append(callArgs, av)
		}
		lastType := fnType.In(numIn - 1)
		if lastType == reflect.TypeOf(&Query{}) {
			callArgs = append(callArgs, reflect.ValueOf(query))
		} else if lastType == reflect.TypeOf(Query{}) {
			callArgs = append(callArgs, reflect.ValueOf(*query))
		} else {
			return
		}
		results := fnVal.Call(callArgs)
		if len(results) > 0 {
			ctx.handleFuncResult(results[0].Interface(), query)
		}
		return
	}
}

// handleFuncResult 处理函数返回值
func (ctx *StaticContext) handleFuncResult(result interface{}, query *Query) {
	if result == nil {
		return
	}
	if s, ok := result.(string); ok {
		query.SQL = s
	} else if q, ok := result.(Query); ok {
		*query = q
	} else if qp, ok := result.(*Query); ok && qp != nil {
		*query = *qp
	}
}

// ==================== Covers / Define ====================

// SwapCovers 交换 covers 映射，返回旧的 covers
func (ctx *StaticContext) SwapCovers(newCovers map[string]func(*StaticContext) error) map[string]func(*StaticContext) error {
	old := ctx.covers
	ctx.covers = newCovers
	return old
}

// PushDefine 进入 define 块，返回完整路径
func (ctx *StaticContext) PushDefine(name string) string {
	ctx.definePath = append(ctx.definePath, name)
	if len(ctx.definePath) > 1 {
		return strings.Join(ctx.definePath, ".")
	}
	return name
}

// PopDefine 离开 define 块
func (ctx *StaticContext) PopDefine() {
	if len(ctx.definePath) > 0 {
		ctx.definePath = ctx.definePath[:len(ctx.definePath)-1]
	}
}

// GetCover 获取 cover 函数
func (ctx *StaticContext) GetCover(fullPath, name string) func(*StaticContext) error {
	if fn, ok := ctx.covers[fullPath]; ok {
		return fn
	}
	if fn, ok := ctx.covers[name]; ok {
		return fn
	}
	return nil
}

// ==================== Use / 动态回退 ====================

// UseTemplate 使用另一个模板（当目标不在同一生成文件中时使用）
func (ctx *StaticContext) UseTemplate(path string, covers map[string]func(*StaticContext) error) error {
	oldCovers := ctx.covers
	ctx.covers = covers
	defer func() { ctx.covers = oldCovers }()

	parts := strings.Split(path, ".")
	if len(parts) < 2 {
		return fmt.Errorf("invalid use path: %s", path)
	}

	fullKey := parts[0] + "." + parts[1]
	defineName := ""
	if len(parts) > 2 {
		defineName = parts[2]
	}

	// 尝试静态版本
	if defineName == "" {
		if fn, ok := getStaticFunc(fullKey); ok {
			return fn(ctx)
		}
	}

	// 回退到动态执行
	return ctx.useDynamic(fullKey, defineName, covers)
}

// useDynamic 动态执行模板（回退机制，仅在目标模板未生成时使用）
func (ctx *StaticContext) useDynamic(key, defineName string, covers map[string]func(*StaticContext) error) error {
	ast, ok := ctx.engine.compiledAST[key]
	if !ok {
		return fmt.Errorf("template not found: %s", key)
	}

	// 创建动态执行上下文（需要解释器）
	execCtx := &executionContext{
		engine:       ctx.engine,
		scope:        ctx.scope,
		covers:       make(map[string][]Node),
		interp:       interpreter.New(),
		scopeObj:     ctx.scopeObj,
		typeInfo:     ctx.typeInfo,
		staticCovers: covers,
	}

	for name, fn := range ctx.engine.funcs {
		execCtx.interp.BindFunc(name, fn)
	}
	for name, value := range ctx.scope {
		if reflect.TypeOf(value) != nil && reflect.TypeOf(value).Kind() == reflect.Func {
			execCtx.interp.BindFunc(name, value)
		}
	}

	if defineName != "" {
		defineNode := findDefine(ast.Nodes, defineName)
		if defineNode == nil {
			return fmt.Errorf("define not found: %s in template %s", defineName, key)
		}
		if err := execCtx.executeNodes(defineNode.Body); err != nil {
			return err
		}
	} else {
		if err := execCtx.executeNodes(ast.Nodes); err != nil {
			return err
		}
	}

	ctx.sql.WriteString(execCtx.sql.String())
	ctx.args = append(ctx.args, execCtx.args...)
	return nil
}

// ExecCode 执行内联代码块（@{} 回退，使用解释器）
func (ctx *StaticContext) ExecCode(code string) error {
	interp := interpreter.New()
	for name, value := range ctx.scope {
		interp.BindValue(name, value)
	}
	wrappedCode := fmt.Sprintf("package main\nfunc main() {\n\t%s\n}", code)
	_, err := interp.Eval(wrappedCode)
	return err
}

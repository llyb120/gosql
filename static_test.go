package gosql

import (
	"os"
	"strings"
	"testing"
)

// ==================== 静态模式核心测试 ====================
// 所有测试使用纯静态方法（无解释器），与生成代码的调用方式完全一致。

// TestStaticBasicSQL 测试静态模式的基础 SQL 生成
func TestStaticBasicSQL(t *testing.T) {
	engine := New()
	err := engine.LoadMarkdown(testMarkdown)
	if err != nil {
		t.Fatalf("LoadMarkdown error: %v", err)
	}

	// 注册静态函数（模拟生成代码）
	RegisterStatic("tpl.sql1", func(ctx *StaticContext) (retErr error) {
		defer ctx.Recover(&retErr)
		ctx.WriteSQL("select * from table \nwhere \n    id = ")
		ctx.AppendArg(ctx.MustGet("id"))
		ctx.WriteSQL("\n    -- 数组的情况\n    and id in (")
		ctx.AppendArg(ctx.MustGet("ids"))
		ctx.WriteSQL(")")
		return nil
	})
	defer func() {
		staticMu.Lock()
		delete(staticFuncs, "tpl.sql1")
		staticMu.Unlock()
	}()

	args := map[string]interface{}{
		"id":  1,
		"ids": []int{1, 2, 3},
	}

	query, err := engine.GetSql("tpl.sql1", args)
	if err != nil {
		t.Fatalf("GetSql error: %v", err)
	}

	t.Logf("SQL: %s", query.SQL)
	t.Logf("Params: %v", query.Params)

	if !strings.Contains(query.SQL, "?") {
		t.Error("SQL should contain placeholders")
	}
	if len(query.Params) != 4 {
		t.Errorf("expected 4 params, got %d", len(query.Params))
	}
}

// TestStaticIfCondition 测试静态模式的 if 条件（纯静态，使用 helpers）
func TestStaticIfCondition(t *testing.T) {
	engine := New()
	err := engine.LoadMarkdown(testMarkdown)
	if err != nil {
		t.Fatalf("LoadMarkdown error: %v", err)
	}

	// 注册使用编译后条件的静态函数（不使用 EvalCondition，使用 GT/LT helpers）
	RegisterStatic("tpl.sql2", func(ctx *StaticContext) (retErr error) {
		defer ctx.Recover(&retErr)
		ctx.WriteSQL("select * from table\nwhere id = 1\n")
		{
			_done := false
			// 编译后的条件: a > 0 → GT(ctx.MustGet("a"), 0)
			if GT(ctx.MustGet("a"), 0) {
				ctx.WriteSQL("    and name = ")
				ctx.AppendArg(ctx.MustGet("name"))
				ctx.WriteSQL("\n")
				_done = true
			}
			// 编译后的条件: a < 0 → LT(ctx.MustGet("a"), 0)
			if !_done && LT(ctx.MustGet("a"), 0) {
				ctx.WriteSQL("    and age = ")
				ctx.AppendArg(ctx.MustGet("age"))
				ctx.WriteSQL("\n")
				_done = true
			}
			if !_done {
				ctx.WriteSQL("    and id = ")
				ctx.AppendArg(ctx.MustGet("id"))
				ctx.WriteSQL("\n")
			}
		}
		return nil
	})
	defer func() {
		staticMu.Lock()
		delete(staticFuncs, "tpl.sql2")
		staticMu.Unlock()
	}()

	// a > 0
	args := map[string]interface{}{"a": 1, "name": "test", "age": 20, "id": 1}
	query, err := engine.GetSql("tpl.sql2", args)
	if err != nil {
		t.Fatalf("GetSql error: %v", err)
	}
	t.Logf("SQL (a > 0): %s", query.SQL)
	if !strings.Contains(query.SQL, "name =") {
		t.Error("SQL should contain 'name =' when a > 0")
	}

	// a < 0
	args["a"] = -1
	query, err = engine.GetSql("tpl.sql2", args)
	if err != nil {
		t.Fatalf("GetSql error: %v", err)
	}
	t.Logf("SQL (a < 0): %s", query.SQL)
	if !strings.Contains(query.SQL, "age =") {
		t.Error("SQL should contain 'age =' when a < 0")
	}

	// a == 0 (else)
	args["a"] = 0
	query, err = engine.GetSql("tpl.sql2", args)
	if err != nil {
		t.Fatalf("GetSql error: %v", err)
	}
	t.Logf("SQL (a == 0): %s", query.SQL)
	if !strings.Contains(query.SQL, "id =") {
		t.Error("SQL should contain 'id =' when a == 0")
	}
}

// TestStaticForRange 测试静态模式的 for range（使用 Range helper）
func TestStaticForRange(t *testing.T) {
	engine := New()
	engine.LoadMarkdown(testMarkdown)

	RegisterStatic("test.sql_for_range", func(ctx *StaticContext) (retErr error) {
		defer ctx.Recover(&retErr)
		ctx.WriteSQL("select * from table where 1 = 1\n")
		// 编译后的 for range: Range(ctx.MustGet("ids"), func(idx, val) { ... })
		if err := Range(ctx.MustGet("ids"), func(idx, val interface{}) error {
			ctx.Set("i", idx)
			ctx.Set("v", val)
			ctx.WriteSQL("    and id = ")
			ctx.AppendArg(ctx.MustGet("v"))
			ctx.WriteSQL("\n")
			return nil
		}); err != nil {
			return err
		}
		return nil
	})
	defer func() {
		staticMu.Lock()
		delete(staticFuncs, "test.sql_for_range")
		staticMu.Unlock()
	}()

	args := map[string]interface{}{
		"ids": []int{10, 20, 30},
	}

	query, err := engine.GetSql("test.sql_for_range", args)
	if err != nil {
		t.Fatalf("GetSql error: %v", err)
	}

	t.Logf("SQL: %s", query.SQL)
	t.Logf("Params: %v", query.Params)

	if strings.Count(query.SQL, "and id =") != 3 {
		t.Errorf("expected 3 'and id =' occurrences, got %d", strings.Count(query.SQL, "and id ="))
	}
	if len(query.Params) != 3 {
		t.Errorf("expected 3 params, got %d", len(query.Params))
	}
}

// TestStaticForTraditional 测试静态模式的传统 for（使用 native Go for + helpers）
func TestStaticForTraditional(t *testing.T) {
	engine := New()
	engine.LoadMarkdown(testMarkdown)

	RegisterStatic("test.sql_for_trad", func(ctx *StaticContext) (retErr error) {
		defer ctx.Recover(&retErr)
		ctx.WriteSQL("select * from table where 1 = 1\n")
		// 编译后的 for: ctx.Set("i", 0); for LT(ctx.MustGet("i"), 3) { ... ctx.Set("i", Add(...)) }
		{
			ctx.Set("i", 0)
			for LT(ctx.MustGet("i"), 3) {
				ctx.WriteSQL("    and col")
				ctx.AppendRaw(ctx.MustGet("i"))
				ctx.WriteSQL(" = ")
				ctx.AppendArg(ctx.MustGet("i"))
				ctx.WriteSQL("\n")
				ctx.Set("i", Add(ctx.MustGet("i"), 1))
			}
		}
		return nil
	})
	defer func() {
		staticMu.Lock()
		delete(staticFuncs, "test.sql_for_trad")
		staticMu.Unlock()
	}()

	args := map[string]interface{}{}
	query, err := engine.GetSql("test.sql_for_trad", args)
	if err != nil {
		t.Fatalf("GetSql error: %v", err)
	}

	t.Logf("SQL: %s", query.SQL)
	t.Logf("Params: %v", query.Params)

	if strings.Count(query.SQL, "col") != 3 {
		t.Errorf("expected 3 'col' occurrences")
	}
	if len(query.Params) != 3 {
		t.Errorf("expected 3 params, got %d", len(query.Params))
	}
}

// TestStaticConditionalVar 测试静态模式的条件变量
func TestStaticConditionalVar(t *testing.T) {
	engine := New()
	engine.LoadMarkdown(testMarkdown)

	RegisterStatic("test.sql_cond_var", func(ctx *StaticContext) (retErr error) {
		defer ctx.Recover(&retErr)
		ctx.WriteSQL("select * from users\nwhere 1 = 1\n    and name = ")
		{
			_v, _ok := ctx.Get("name")
			if !_ok || !ToBool(_v) {
				ctx.SkipCurrentLine()
			} else {
				ctx.AppendArg(_v)
			}
		}
		ctx.WriteSQL("\n    and notexist = ")
		{
			_v, _ok := ctx.Get("notexist")
			if !_ok || !ToBool(_v) {
				ctx.SkipCurrentLine()
			} else {
				ctx.AppendArg(_v)
			}
		}
		ctx.WriteSQL("\n")
		return nil
	})
	defer func() {
		staticMu.Lock()
		delete(staticFuncs, "test.sql_cond_var")
		staticMu.Unlock()
	}()

	args := map[string]interface{}{
		"name": "test",
	}

	query, err := engine.GetSql("test.sql_cond_var", args)
	if err != nil {
		t.Fatalf("GetSql error: %v", err)
	}

	t.Logf("SQL: %s", query.SQL)
	t.Logf("Params: %v", query.Params)

	if !strings.Contains(query.SQL, "name =") {
		t.Error("SQL should contain 'name ='")
	}
	if strings.Contains(query.SQL, "notexist =") {
		t.Error("SQL should NOT contain 'notexist ='")
	}
}

// TestStaticStructArgs 测试静态模式的结构体参数
func TestStaticStructArgs(t *testing.T) {
	engine := New()
	markdown := `
# user
## findById
` + "```sql" + `
select * from users where id = @id and name = @name
` + "```" + `
`
	engine.LoadMarkdown(markdown)

	RegisterStatic("user.findById", func(ctx *StaticContext) (retErr error) {
		defer ctx.Recover(&retErr)
		ctx.WriteSQL("select * from users where id = ")
		ctx.AppendArg(ctx.MustGet("id"))
		ctx.WriteSQL(" and name = ")
		ctx.AppendArg(ctx.MustGet("name"))
		return nil
	})
	defer func() {
		staticMu.Lock()
		delete(staticFuncs, "user.findById")
		staticMu.Unlock()
	}()

	type Args struct {
		Id   int
		Name string
	}

	query, err := engine.GetSql("user.findById", Args{Id: 1, Name: "test"})
	if err != nil {
		t.Fatalf("GetSql error: %v", err)
	}

	t.Logf("SQL: %s", query.SQL)
	t.Logf("Params: %v", query.Params)

	if len(query.Params) != 2 {
		t.Errorf("expected 2 params, got %d", len(query.Params))
	}
}

// TestStaticCall 测试 Call 方法（纯反射函数调用）
func TestStaticCall(t *testing.T) {
	engine := New()
	engine.LoadMarkdown(testMarkdown)

	RegisterStatic("test.sql_call", func(ctx *StaticContext) (retErr error) {
		defer ctx.Recover(&retErr)
		ctx.WriteSQL("name is ")
		// 编译后: ctx.Call("GetName") 代替 ctx.EvalExpr("GetName()")
		ctx.AppendRaw(ctx.Call("GetName"))
		return nil
	})
	defer func() {
		staticMu.Lock()
		delete(staticFuncs, "test.sql_call")
		staticMu.Unlock()
	}()

	type Args struct {
		Id int
	}
	type argsWithMethod struct {
		Args
	}
	a := &argsWithMethod{Args: Args{Id: 1}}

	// 通过引擎注册一个函数
	engine.RegisterFunc("GetName", func() string { return "hello" })

	query, err := engine.GetSql("test.sql_call", a)
	if err != nil {
		t.Fatalf("GetSql error: %v", err)
	}
	t.Logf("SQL: %s", query.SQL)
	if !strings.Contains(query.SQL, "name is hello") {
		t.Error("SQL should contain 'name is hello'")
	}
}

// TestStaticFuncBlock 测试 CallFuncBlock 方法
func TestStaticFuncBlock(t *testing.T) {
	engine := New()
	engine.LoadMarkdown(testMarkdown)

	// 注册 trim 函数
	engine.RegisterFunc("trim", func(operator string, query *Query) {
		sql := strings.TrimSpace(query.SQL)
		sql = strings.TrimPrefix(sql, operator)
		sql = strings.TrimSpace(sql)
		query.SQL = sql
	})

	RegisterStatic("test.sql_funcblock", func(ctx *StaticContext) (retErr error) {
		defer ctx.Recover(&retErr)
		ctx.WriteSQL("select * from table where 1 = 1\n")
		{
			// 编译后的 @trim("and") { ... }
			// 用变量遮蔽让 body 中的 ctx 写入子上下文
			_outerCtx := ctx
			ctx = _outerCtx.SubContext()
			if err := Range(ctx.MustGet("ids"), func(_idx, _val interface{}) error {
				ctx.Set("v", _val)
				ctx.WriteSQL("        and id = ")
				ctx.AppendArg(ctx.MustGet("v"))
				ctx.WriteSQL("\n")
				return nil
			}); err != nil {
				return err
			}
			_query := ctx.BuildPtr()
			ctx = _outerCtx
			ctx.CallFuncBlock("trim", _query, "and")
			ctx.WriteQuery(_query)
		}
		return nil
	})
	defer func() {
		staticMu.Lock()
		delete(staticFuncs, "test.sql_funcblock")
		staticMu.Unlock()
	}()

	args := map[string]interface{}{
		"ids": []int{1, 2, 3},
	}
	query, err := engine.GetSql("test.sql_funcblock", args)
	if err != nil {
		t.Fatalf("GetSql error: %v", err)
	}
	t.Logf("SQL: %s", query.SQL)
	t.Logf("Params: %v", query.Params)

	// trim 应该去掉第一个 "and"
	if strings.Contains(query.SQL, "1 = 1\nand") {
		t.Error("trim should have removed leading 'and'")
	}
	if len(query.Params) != 3 {
		t.Errorf("expected 3 params, got %d", len(query.Params))
	}
}

// TestStaticUseTemplate 测试静态模式的 use/cover（直接函数调用）
func TestStaticUseTemplate(t *testing.T) {
	engine := New()
	err := engine.LoadMarkdown(testMarkdown)
	if err != nil {
		t.Fatalf("LoadMarkdown error: %v", err)
	}

	// define a 作为独立函数
	sql4DefineA := func(ctx *StaticContext) error {
		_fullPath := ctx.PushDefine("a")
		defer ctx.PopDefine()
		if _coverFn := ctx.GetCover(_fullPath, "a"); _coverFn != nil {
			return _coverFn(ctx)
		}
		// 默认内容
		ctx.WriteSQL("    and id = ")
		ctx.AppendArg(ctx.MustGet("id"))
		ctx.WriteSQL("\n")
		return nil
	}

	// sql4 模板函数
	sql4Fn := func(ctx *StaticContext) (retErr error) {
		defer ctx.Recover(&retErr)
		ctx.WriteSQL("select * from table\nwhere 1 = 1\n")
		if err := sql4DefineA(ctx); err != nil {
			return err
		}
		return nil
	}
	RegisterStatic("tpl.sql4", sql4Fn)

	// sql3 模板函数（use sql4，cover a）
	RegisterStatic("tpl.sql3", func(ctx *StaticContext) (retErr error) {
		defer ctx.Recover(&retErr)
		ctx.WriteSQL("select * from \n")
		{
			// @use test.sql4 { @cover a { ... } }
			_covers := make(map[string]func(*StaticContext) error)
			_covers["a"] = func(ctx *StaticContext) error {
				ctx.WriteSQL("    and id <> ")
				ctx.AppendArg(ctx.MustGet("id"))
				ctx.WriteSQL("\n")
				return nil
			}
			// 直接函数调用（不走 UseTemplate）
			_oldCovers := ctx.SwapCovers(_covers)
			if err := sql4Fn(ctx); err != nil {
				ctx.SwapCovers(_oldCovers)
				return err
			}
			ctx.SwapCovers(_oldCovers)
		}
		return nil
	})
	defer func() {
		staticMu.Lock()
		delete(staticFuncs, "tpl.sql3")
		delete(staticFuncs, "tpl.sql4")
		staticMu.Unlock()
	}()

	args := map[string]interface{}{"id": 1}
	query, err := engine.GetSql("tpl.sql3", args)
	if err != nil {
		t.Fatalf("GetSql error: %v", err)
	}

	t.Logf("SQL: %s", query.SQL)
	t.Logf("Params: %v", query.Params)

	if !strings.Contains(query.SQL, "id <>") {
		t.Error("SQL should contain covered content 'id <>'")
	}
	if strings.Contains(query.SQL, "and id =") && !strings.Contains(query.SQL, "id <>") {
		t.Error("Cover should override default define content")
	}
}

// TestStaticDefinePathFallback 测试 define 路径访问回退到动态模式
func TestStaticDefinePathFallback(t *testing.T) {
	engine := New()
	err := engine.LoadMarkdown(testMarkdown)
	if err != nil {
		t.Fatalf("LoadMarkdown error: %v", err)
	}

	// 不注册任何静态函数，define 路径应回退到动态模式
	args := map[string]interface{}{"id": 1}
	query, err := engine.GetSql("tpl.sql4", args)
	if err != nil {
		t.Fatalf("GetSql error: %v", err)
	}

	t.Logf("SQL: %s", query.SQL)
	if !strings.Contains(query.SQL, "and id =") {
		t.Error("Dynamic fallback should work for define paths")
	}
}

// TestStaticFallbackDynamic 测试静态不存在时回退到动态
func TestStaticFallbackDynamic(t *testing.T) {
	engine := New()
	err := engine.LoadMarkdown(testMarkdown)
	if err != nil {
		t.Fatalf("LoadMarkdown error: %v", err)
	}

	// 不注册静态函数，应该使用动态模式
	args := map[string]interface{}{"id": 1, "ids": []int{1, 2, 3}}
	query, err := engine.GetSql("tpl.sql1", args)
	if err != nil {
		t.Fatalf("GetSql error: %v", err)
	}

	if !strings.Contains(query.SQL, "?") {
		t.Error("Dynamic fallback should work")
	}
}

// ==================== 表达式编译器测试 ====================

func TestExprCompiler(t *testing.T) {
	tests := []struct {
		expr       string
		expectCode string
		expectBool bool
	}{
		{"a > 0", `GT(ctx.MustGet("a"), 0)`, true},
		{"a < 0", `LT(ctx.MustGet("a"), 0)`, true},
		{"a >= 10", `GE(ctx.MustGet("a"), 10)`, true},
		{"a <= 10", `LE(ctx.MustGet("a"), 10)`, true},
		{"a == 0", `EQ(ctx.MustGet("a"), 0)`, true},
		{"a != 0", `NE(ctx.MustGet("a"), 0)`, true},
		{"a > 0 && b < 10", `GT(ctx.MustGet("a"), 0) && LT(ctx.MustGet("b"), 10)`, true},
		{"a > 0 || b > 0", `GT(ctx.MustGet("a"), 0) || GT(ctx.MustGet("b"), 0)`, true},
		{`GetName()`, `ctx.Call("GetName")`, false},
		{`trim("and")`, `ctx.Call("trim", "and")`, false},
		{"true", "true", true},
		{"false", "false", true},
		{"name", `ctx.MustGet("name")`, false},
		{"0", "0", false},
		{`"hello"`, `"hello"`, false},
		{"-1", "-1", false},
		{"!flag", `!ToBool(ctx.MustGet("flag"))`, true},
		{"a + 1", `Add(ctx.MustGet("a"), 1)`, false},
		{`len(ids) > 0`, `GT(CallLen(ctx.MustGet("ids")), 0)`, true},
	}

	for _, tt := range tests {
		code, isBool, err := CompileExprToGo(tt.expr, "")
		if err != nil {
			t.Errorf("CompileExprToGo(%q) error: %v", tt.expr, err)
			continue
		}
		if code != tt.expectCode {
			t.Errorf("CompileExprToGo(%q) = %q, want %q", tt.expr, code, tt.expectCode)
		}
		if isBool != tt.expectBool {
			t.Errorf("CompileExprToGo(%q) isBool = %v, want %v", tt.expr, isBool, tt.expectBool)
		}
	}
}

func TestCompileConditionToGo(t *testing.T) {
	// 非 bool 表达式应该被包裹在 ToBool 中
	code, err := CompileConditionToGo("name", "gosql.")
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if code != `gosql.ToBool(ctx.MustGet("name"))` {
		t.Errorf("got %q", code)
	}

	// bool 表达式不需要包裹
	code, err = CompileConditionToGo("a > 0", "gosql.")
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if code != `gosql.GT(ctx.MustGet("a"), 0)` {
		t.Errorf("got %q", code)
	}

	// 空表达式 → false
	code, err = CompileConditionToGo("", "")
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if code != "false" {
		t.Errorf("got %q", code)
	}
}

func TestParseForRange(t *testing.T) {
	info, err := ParseForRange("i, v := range ids", "gosql.")
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if info.IndexVar != "i" || info.ValueVar != "v" {
		t.Errorf("got IndexVar=%q, ValueVar=%q", info.IndexVar, info.ValueVar)
	}
	if info.RangeCode != `ctx.MustGet("ids")` {
		t.Errorf("got RangeCode=%q", info.RangeCode)
	}

	info, err = ParseForRange("_, v := range GetItems()", "")
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if info.IndexVar != "_" || info.ValueVar != "v" {
		t.Errorf("got IndexVar=%q, ValueVar=%q", info.IndexVar, info.ValueVar)
	}
	if info.RangeCode != `ctx.Call("GetItems")` {
		t.Errorf("got RangeCode=%q", info.RangeCode)
	}
}

func TestParseForInit(t *testing.T) {
	info, err := ParseForInit("i := 0", "gosql.")
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if info.VarName != "i" || info.ValueCode != "0" {
		t.Errorf("got VarName=%q, ValueCode=%q", info.VarName, info.ValueCode)
	}
}

func TestParseForPost(t *testing.T) {
	info, err := ParseForPost("i++", "gosql.")
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if info.VarName != "i" {
		t.Errorf("got VarName=%q", info.VarName)
	}
	if !strings.Contains(info.Code, "Add") {
		t.Errorf("i++ should use Add, got %q", info.Code)
	}
}

func TestParseFuncCallExpr(t *testing.T) {
	info, err := ParseFuncCallExpr(`trim("and")`, "gosql.")
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if info.FuncName != "trim" {
		t.Errorf("got FuncName=%q", info.FuncName)
	}
	if len(info.Args) != 1 || info.Args[0] != `"and"` {
		t.Errorf("got Args=%v", info.Args)
	}

	info, err = ParseFuncCallExpr("Test()", "")
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if info.FuncName != "Test" || len(info.Args) != 0 {
		t.Errorf("got FuncName=%q, Args=%v", info.FuncName, info.Args)
	}
}

// ==================== Helpers 测试 ====================

func TestHelpers(t *testing.T) {
	// GT/LT/GE/LE/EQ/NE
	if !GT(5, 3) {
		t.Error("GT(5,3) should be true")
	}
	if !LT(3, 5) {
		t.Error("LT(3,5) should be true")
	}
	if !GE(5, 5) {
		t.Error("GE(5,5) should be true")
	}
	if !LE(5, 5) {
		t.Error("LE(5,5) should be true")
	}
	if !EQ(5, 5) {
		t.Error("EQ(5,5) should be true")
	}
	if !NE(5, 3) {
		t.Error("NE(5,3) should be true")
	}

	// 跨类型比较
	if !GT(int64(5), 3) {
		t.Error("GT(int64(5), 3) should be true")
	}
	if !EQ(1, 1.0) {
		t.Error("EQ(1, 1.0) should be true")
	}

	// ToBool
	if !ToBool(1) {
		t.Error("ToBool(1) should be true")
	}
	if ToBool(0) {
		t.Error("ToBool(0) should be false")
	}
	if !ToBool("hello") {
		t.Error("ToBool('hello') should be true")
	}
	if ToBool("") {
		t.Error("ToBool('') should be false")
	}
	if ToBool(nil) {
		t.Error("ToBool(nil) should be false")
	}

	// Add/Sub
	if Add(3, 4) != 7 {
		t.Error("Add(3,4) should be 7")
	}
	if Sub(10, 3) != 7 {
		t.Error("Sub(10,3) should be 7")
	}

	// Range
	count := 0
	Range([]int{1, 2, 3}, func(idx, val interface{}) error {
		count++
		return nil
	})
	if count != 3 {
		t.Errorf("Range count should be 3, got %d", count)
	}

	// CallLen
	if CallLen([]int{1, 2, 3}) != 3 {
		t.Error("CallLen should return 3")
	}
	if CallLen("hello") != 5 {
		t.Error("CallLen('hello') should return 5")
	}
}

// ==================== 代码生成器测试 ====================

func TestCodeGenerator(t *testing.T) {
	templates, err := ParseMarkdown(testMarkdown)
	if err != nil {
		t.Fatalf("ParseMarkdown error: %v", err)
	}

	asts := make(map[string]*TemplateAST)
	for _, tmpl := range templates {
		key := tmpl.Namespace + "." + tmpl.Name
		ast, err := ParseTemplate(tmpl.Content)
		if err != nil {
			t.Fatalf("ParseTemplate %s error: %v", key, err)
		}
		ast.Namespace = tmpl.Namespace
		ast.Name = tmpl.Name
		asts[key] = ast
	}

	gen := NewCodeGenerator("gosql.")
	code, err := gen.GenerateFile(templates, asts, "mypackage", "test.md")
	if err != nil {
		t.Fatalf("GenerateFile error: %v", err)
	}

	t.Logf("Generated code length: %d bytes", len(code))

	// 验证生成代码包含必要的结构
	checks := []struct {
		desc    string
		content string
	}{
		{"package declaration", "package mypackage"},
		{"init function", "func init()"},
		{"register static", "gosql.RegisterStatic"},
		{"StaticContext", "gosql.StaticContext"},
		{"panic recovery", "ctx.Recover(&retErr)"},
		{"DO NOT EDIT", "DO NOT EDIT"},
		{"tpl.sql1 registration", `gosql.RegisterStatic("tpl.sql1"`},
		{"tpl.sql2 registration", `gosql.RegisterStatic("tpl.sql2"`},
		{"WriteSQL calls", "ctx.WriteSQL"},
		{"AppendArg calls", "ctx.AppendArg"},
		{"MustGet calls", "ctx.MustGet"},
		{"PushDefine calls", "ctx.PushDefine"},
	}

	for _, check := range checks {
		if !strings.Contains(code, check.content) {
			t.Errorf("Should contain %s: %q", check.desc, check.content)
		}
	}

	// 验证使用的是编译后的比较函数而不是 EvalCondition
	if strings.Contains(code, "EvalCondition") {
		t.Error("Generated code should NOT contain EvalCondition (should use compiled conditions)")
	}
	if strings.Contains(code, "EvalExpr") {
		t.Error("Generated code should NOT contain EvalExpr (should use compiled expressions)")
	}
	if strings.Contains(code, "ForTraditional") || strings.Contains(code, "ForRange") {
		t.Error("Generated code should NOT contain ForTraditional/ForRange methods (should use native Go loops)")
	}

	// 验证使用了静态比较 helpers
	if !strings.Contains(code, "gosql.GT") && !strings.Contains(code, "gosql.LT") {
		t.Error("Generated code should use gosql.GT/LT for conditions")
	}
	// 验证传统 for 循环被编译为原生 Go for
	if !strings.Contains(code, "for gosql.LT") {
		t.Error("Generated code should compile for loop to native Go for + gosql.LT")
	}
}

func TestCodeGeneratorFromExampleMd(t *testing.T) {
	bs, err := os.ReadFile("example.md")
	if err != nil {
		t.Skipf("example.md not found: %v", err)
	}

	templates, err := ParseMarkdown(string(bs))
	if err != nil {
		t.Fatalf("ParseMarkdown error: %v", err)
	}

	asts := make(map[string]*TemplateAST)
	for _, tmpl := range templates {
		key := tmpl.Namespace + "." + tmpl.Name
		ast, err := ParseTemplate(tmpl.Content)
		if err != nil {
			t.Fatalf("ParseTemplate %s error: %v", key, err)
		}
		ast.Namespace = tmpl.Namespace
		ast.Name = tmpl.Name
		asts[key] = ast
	}

	gen := NewCodeGenerator("gosql.")
	code, err := gen.GenerateFile(templates, asts, "example", "example.md")
	if err != nil {
		t.Fatalf("GenerateFile error: %v", err)
	}

	t.Logf("Generated %d bytes of code from example.md (%d templates)", len(code), len(templates))

	// 验证所有模板都已生成
	for _, tmpl := range templates {
		key := tmpl.Namespace + "." + tmpl.Name
		if !strings.Contains(code, `gosql.RegisterStatic("`+key+`"`) {
			t.Errorf("Missing registration for %s", key)
		}
	}

	// 验证 define 编译为独立函数
	if !strings.Contains(code, "ctx.PushDefine") {
		t.Error("Should contain PushDefine in define functions")
	}
	if !strings.Contains(code, "ctx.GetCover") {
		t.Error("Should contain GetCover in define functions")
	}

	// 验证 use 编译为直接调用或 UseTemplate
	if !strings.Contains(code, "ctx.SwapCovers") && !strings.Contains(code, "ctx.UseTemplate") {
		t.Error("Should contain SwapCovers or UseTemplate for @use")
	}

	// 验证 FuncBlock 使用变量遮蔽 + CallFuncBlock
	if !strings.Contains(code, ".SubContext()") {
		t.Error("Should contain SubContext for func blocks")
	}
	if !strings.Contains(code, "ctx.CallFuncBlock") {
		t.Error("Should contain CallFuncBlock for func blocks")
	}

	// 验证不再使用动态方法
	if strings.Contains(code, "ctx.EvalCondition") {
		t.Error("Should NOT contain EvalCondition")
	}
	if strings.Contains(code, "ctx.EvalExpr") {
		t.Error("Should NOT contain EvalExpr")
	}
}

// ==================== Benchmark ====================

func BenchmarkDynamicMode(b *testing.B) {
	engine := New()
	engine.LoadMarkdown(testMarkdown)

	args := map[string]interface{}{
		"id":  1,
		"ids": []int{1, 2, 3},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := engine.GetSql("tpl.sql1", args)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkStaticMode(b *testing.B) {
	engine := New()
	engine.LoadMarkdown(testMarkdown)

	RegisterStatic("test.sql1_bench", func(ctx *StaticContext) (retErr error) {
		defer ctx.Recover(&retErr)
		ctx.WriteSQL("select * from table \nwhere \n    id = ")
		ctx.AppendArg(ctx.MustGet("id"))
		ctx.WriteSQL("\n    -- 数组的情况\n    and id in (")
		ctx.AppendArg(ctx.MustGet("ids"))
		ctx.WriteSQL(")\n    -- 直接输出的情况\n    and id = ")
		ctx.AppendRaw(ctx.MustGet("id"))
		return nil
	})
	defer func() {
		staticMu.Lock()
		delete(staticFuncs, "test.sql1_bench")
		staticMu.Unlock()
	}()

	args := map[string]interface{}{
		"id":  1,
		"ids": []int{1, 2, 3},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := engine.GetSql("test.sql1_bench", args)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkStaticModeComplex(b *testing.B) {
	engine := New()
	engine.LoadMarkdown(testMarkdown)

	// 使用纯静态编译后代码（无解释器）
	RegisterStatic("test.sql2_bench", func(ctx *StaticContext) (retErr error) {
		defer ctx.Recover(&retErr)
		ctx.WriteSQL("select * from table\nwhere id = 1\n")
		{
			_done := false
			if GT(ctx.MustGet("a"), 0) {
				ctx.WriteSQL("    and name = ")
				ctx.AppendArg(ctx.MustGet("name"))
				ctx.WriteSQL("\n")
				_done = true
			}
			if !_done && LT(ctx.MustGet("a"), 0) {
				ctx.WriteSQL("    and age = ")
				ctx.AppendArg(ctx.MustGet("age"))
				ctx.WriteSQL("\n")
				_done = true
			}
			if !_done {
				ctx.WriteSQL("    and id = ")
				ctx.AppendArg(ctx.MustGet("id"))
				ctx.WriteSQL("\n")
			}
		}
		return nil
	})
	defer func() {
		staticMu.Lock()
		delete(staticFuncs, "test.sql2_bench")
		staticMu.Unlock()
	}()

	args := map[string]interface{}{
		"a":    1,
		"name": "test",
		"age":  20,
		"id":   1,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := engine.GetSql("test.sql2_bench", args)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkDynamicModeComplex(b *testing.B) {
	engine := New()
	engine.LoadMarkdown(testMarkdown)

	args := map[string]interface{}{
		"a":    1,
		"name": "test",
		"age":  20,
		"id":   1,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := engine.GetSql("tpl.sql2", args)
		if err != nil {
			b.Fatal(err)
		}
	}
}

package gosql

import (
	"os"
	"strings"
	"testing"
)

// 测试 markdown
const testMarkdown = `
# test

## sql1
基础功能
` + "```sql" + `
select * from table 
where 
    id = @id
    -- 数组的情况
    and id in (@ids)
` + "```" + `

## sql2 
流程控制
` + "```sql" + `
select * from table
where id = 1
@if a > 0 {
    and name = @name
} else if a < 0 {
    and age = @age
} else {
    and id = @id
}
` + "```" + `

## sql3
use function
` + "```sql" + `
select * from 
@use test.sql4 {
    @cover a {
        and id <> @id
    }
}
` + "```" + `

## sql4
define 
` + "```sql" + `
select * from table
where 1 = 1
@define a {
    and id = @id
}
` + "```" + `

## sql5
for loop
` + "```sql" + `
select * from table
where 1 = 1
@for i := 0; i < 3; i++ {
    and col@=i@ = @i
}
` + "```" + `
`

func TestParseMarkdown(t *testing.T) {
	templates, err := ParseMarkdown(testMarkdown)
	if err != nil {
		t.Fatalf("ParseMarkdown error: %v", err)
	}

	if len(templates) != 5 {
		t.Errorf("expected 5 templates, got %d", len(templates))
	}

	// 验证第一个模板
	if templates[0].Namespace != "test" {
		t.Errorf("expected namespace 'test', got '%s'", templates[0].Namespace)
	}
	if templates[0].Name != "sql1" {
		t.Errorf("expected name 'sql1', got '%s'", templates[0].Name)
	}
}

func TestLexer(t *testing.T) {
	input := `select * from table where id = @id and name = @name`
	lexer := NewLexer(input)
	tokens, err := lexer.Tokenize()
	if err != nil {
		t.Fatalf("Lexer error: %v", err)
	}

	// 应该有 TEXT, VAR, TEXT, VAR, EOF
	expectedTypes := []TokenType{TOKEN_TEXT, TOKEN_VAR, TOKEN_TEXT, TOKEN_VAR, TOKEN_EOF}
	if len(tokens) != len(expectedTypes) {
		t.Errorf("expected %d tokens, got %d", len(expectedTypes), len(tokens))
	}

	for i, expected := range expectedTypes {
		if tokens[i].Type != expected {
			t.Errorf("token %d: expected %s, got %s", i, expected.String(), tokens[i].Type.String())
		}
	}
}

func TestLexerIf(t *testing.T) {
	input := `@if a > 0 {
    and name = @name
} else {
    and id = @id
}`
	lexer := NewLexer(input)
	tokens, err := lexer.Tokenize()
	if err != nil {
		t.Fatalf("Lexer error: %v", err)
	}

	t.Logf("Tokens: %+v", tokens)

	// 验证 token 类型
	if tokens[0].Type != TOKEN_IF {
		t.Errorf("expected IF, got %s", tokens[0].Type.String())
	}
	if tokens[0].Value != "a > 0" {
		t.Errorf("expected 'a > 0', got '%s'", tokens[0].Value)
	}
}

func TestLexerFor(t *testing.T) {
	input := `@for i := 0; i < 10; i++ {
    and id = @id
}`
	lexer := NewLexer(input)
	tokens, err := lexer.Tokenize()
	if err != nil {
		t.Fatalf("Lexer error: %v", err)
	}

	if tokens[0].Type != TOKEN_FOR {
		t.Errorf("expected FOR, got %s", tokens[0].Type.String())
	}
	if tokens[0].Value != "i := 0; i < 10; i++" {
		t.Errorf("expected 'i := 0; i < 10; i++', got '%s'", tokens[0].Value)
	}
}

func TestParser(t *testing.T) {
	input := `select * from table where id = @id
@if a > 0 {
    and name = @name
}`
	ast, err := ParseTemplate(input)
	if err != nil {
		t.Fatalf("ParseTemplate error: %v", err)
	}

	t.Logf("Parsed %d nodes", len(ast.Nodes))
	for i, node := range ast.Nodes {
		t.Logf("  Node %d: %T", i, node)
	}

	// 验证至少有文本节点、变量节点和 if 节点
	hasText := false
	hasVar := false
	hasIf := false
	for _, node := range ast.Nodes {
		switch node.(type) {
		case *TextNode:
			hasText = true
		case *VarNode:
			hasVar = true
		case *IfNode:
			hasIf = true
		}
	}

	if !hasText {
		t.Error("expected at least one TextNode")
	}
	if !hasVar {
		t.Error("expected at least one VarNode")
	}
	if !hasIf {
		t.Error("expected at least one IfNode")
	}
}

func TestBasicSQL(t *testing.T) {
	engine := New()
	err := engine.LoadMarkdown(testMarkdown)
	if err != nil {
		t.Fatalf("LoadMarkdown error: %v", err)
	}

	// 测试基础 SQL
	args := map[string]interface{}{
		"id":  1,
		"ids": []int{1, 2, 3},
	}

	query, err := engine.GetSql("test.sql1", args)
	if err != nil {
		t.Fatalf("GetSql error: %v", err)
	}

	t.Logf("SQL: %s", query.SQL)
	t.Logf("Params: %v", query.Params)

	// 验证 SQL 包含 ?
	if !strings.Contains(query.SQL, "?") {
		t.Errorf("SQL should contain placeholders")
	}

	// 验证参数数量
	if len(query.Params) != 4 { // 1 个 id + 3 个 ids
		t.Errorf("expected 4 params, got %d", len(query.Params))
	}
}

func TestIfCondition(t *testing.T) {
	engine := New()
	err := engine.LoadMarkdown(testMarkdown)
	if err != nil {
		t.Fatalf("LoadMarkdown error: %v", err)
	}

	// 测试 a > 0 的情况
	args := map[string]interface{}{
		"a":    1,
		"name": "test",
		"age":  20,
		"id":   1,
	}

	query, err := engine.GetSql("test.sql2", args)
	if err != nil {
		t.Fatalf("GetSql error: %v", err)
	}

	t.Logf("SQL (a > 0): %s", query.SQL)
	t.Logf("Params: %v", query.Params)

	if !strings.Contains(query.SQL, "name =") {
		t.Errorf("SQL should contain 'name =' when a > 0")
	}

	// 测试 a < 0 的情况
	args["a"] = -1
	query, err = engine.GetSql("test.sql2", args)
	if err != nil {
		t.Fatalf("GetSql error: %v", err)
	}

	t.Logf("SQL (a < 0): %s", query.SQL)
	t.Logf("Params: %v", query.Params)

	if !strings.Contains(query.SQL, "age =") {
		t.Errorf("SQL should contain 'age =' when a < 0")
	}

	// 测试 a == 0 的情况（else）
	args["a"] = 0
	query, err = engine.GetSql("test.sql2", args)
	if err != nil {
		t.Fatalf("GetSql error: %v", err)
	}

	t.Logf("SQL (a == 0): %s", query.SQL)
	t.Logf("Params: %v", query.Params)

	if !strings.Contains(query.SQL, "id =") {
		t.Errorf("SQL should contain 'id =' when a == 0")
	}
}

func TestDefine(t *testing.T) {
	engine := New()
	err := engine.LoadMarkdown(testMarkdown)
	if err != nil {
		t.Fatalf("LoadMarkdown error: %v", err)
	}

	args := map[string]interface{}{
		"id": 1,
	}

	query, err := engine.GetSql("test.sql4", args)
	if err != nil {
		t.Fatalf("GetSql error: %v", err)
	}

	t.Logf("SQL: %s", query.SQL)
	t.Logf("Params: %v", query.Params)

	// 验证 define 块的内容被输出
	if !strings.Contains(query.SQL, "and id =") {
		t.Errorf("SQL should contain define block content")
	}
}

func TestUseAndCover(t *testing.T) {
	engine := New()
	err := engine.LoadMarkdown(testMarkdown)
	if err != nil {
		t.Fatalf("LoadMarkdown error: %v", err)
	}

	args := map[string]interface{}{
		"id": 1,
	}

	query, err := engine.GetSql("test.sql3", args)
	if err != nil {
		t.Fatalf("GetSql error: %v", err)
	}

	t.Logf("SQL: %s", query.SQL)
	t.Logf("Params: %v", query.Params)

	// 验证 cover 覆盖了 define
	if !strings.Contains(query.SQL, "id <>") {
		t.Errorf("SQL should contain covered content 'id <>'")
	}
}

func TestForLoop(t *testing.T) {
	engine := New()
	err := engine.LoadMarkdown(testMarkdown)
	if err != nil {
		t.Fatalf("LoadMarkdown error: %v", err)
	}

	args := map[string]interface{}{}

	query, err := engine.GetSql("test.sql5", args)
	if err != nil {
		t.Fatalf("GetSql error: %v", err)
	}

	t.Logf("SQL: %s", query.SQL)
	t.Logf("Params: %v", query.Params)

	// 验证循环执行了 3 次
	if strings.Count(query.SQL, "col") != 3 {
		t.Errorf("SQL should contain 3 'col' occurrences")
	}
}

func TestStructArgs(t *testing.T) {
	engine := New()

	markdown := `
# user

## findById
` + "```sql" + `
select * from users where id = @id and name = @name
` + "```" + `
`

	err := engine.LoadMarkdown(markdown)
	if err != nil {
		t.Fatalf("LoadMarkdown error: %v", err)
	}

	// 使用结构体作为参数
	type Args struct {
		Id   int
		Name string
	}

	args := Args{
		Id:   1,
		Name: "test",
	}

	query, err := engine.GetSql("user.findById", args)
	if err != nil {
		t.Fatalf("GetSql error: %v", err)
	}

	t.Logf("SQL: %s", query.SQL)
	t.Logf("Params: %v", query.Params)

	if len(query.Params) != 2 {
		t.Errorf("expected 2 params, got %d", len(query.Params))
	}
}

func TestRawOutput(t *testing.T) {
	engine := New()

	markdown := `
# test

## dynamic
` + "```sql" + `
select * from @=tableName@ where id = @id
` + "```" + `
`

	err := engine.LoadMarkdown(markdown)
	if err != nil {
		t.Fatalf("LoadMarkdown error: %v", err)
	}

	args := map[string]interface{}{
		"tableName": "users",
		"id":        1,
	}

	query, err := engine.GetSql("test.dynamic", args)
	if err != nil {
		t.Fatalf("GetSql error: %v", err)
	}

	t.Logf("SQL: %s", query.SQL)
	t.Logf("Params: %v", query.Params)

	// 验证 tableName 被直接输出
	if !strings.Contains(query.SQL, "from users") {
		t.Errorf("SQL should contain 'from users'")
	}

	// 验证 id 是参数
	if len(query.Params) != 1 {
		t.Errorf("expected 1 param, got %d", len(query.Params))
	}
}

func TestNestedDefine(t *testing.T) {
	engine := New()

	markdown := `
# test

## nested
` + "```sql" + `
select * from table
@define outer {
    where 1 = 1
    @define inner {
        and id = @id
    }
}
` + "```" + `

## useNested
` + "```sql" + `
@use test.nested {
    @cover inner {
        and name = @name
    }
}
` + "```" + `
`

	err := engine.LoadMarkdown(markdown)
	if err != nil {
		t.Fatalf("LoadMarkdown error: %v", err)
	}

	args := map[string]interface{}{
		"id":   1,
		"name": "test",
	}

	query, err := engine.GetSql("test.useNested", args)
	if err != nil {
		t.Fatalf("GetSql error: %v", err)
	}

	t.Logf("SQL: %s", query.SQL)
	t.Logf("Params: %v", query.Params)

	// 验证嵌套 define 被覆盖
	if !strings.Contains(query.SQL, "name =") {
		t.Errorf("SQL should contain 'name =' (covered)")
	}
}

func TestErrorMessage(t *testing.T) {
	engine := New()

	// 测试语法错误
	markdown := `
# test

## error
` + "```sql" + `
select * from table
@if a > 0 
    missing brace
}
` + "```" + `
`

	err := engine.LoadMarkdown(markdown)
	if err == nil {
		t.Error("expected error for invalid syntax")
	} else {
		t.Logf("Error message: %v", err)
	}
}

func TestGetSql(t *testing.T) {
	engine := New()
	bs, _ := os.ReadFile("example.md")
	err := engine.LoadMarkdown(string(bs))
	if err != nil {
		t.Fatalf("LoadMarkdown error: %v", err)
	}

	query, err := engine.GetSql("test.sql1", map[string]interface{}{"id": 1, "ids": []int{1, 2, 3}, "trim": func(operator string, query Query) string {
		query.SQL = strings.TrimSpace(query.SQL)
		// trim operator
		query.SQL = strings.TrimPrefix(query.SQL, operator)
		query.SQL = strings.TrimSuffix(query.SQL, operator)
		return " " + query.SQL + " "
	}})
	if err != nil {
		t.Fatalf("GetSql error: %v", err)
	}
	t.Logf("SQL: %s", query.SQL)
	t.Logf("Params: %v", query.Params)

	t.Log("==================")
	t.Log("TestGetSql with struct")
	p := Person{
		Id:   1,
		Name: "test",
	}
	query, err = engine.GetSql("test.sql5", p)
	if err != nil {
		t.Fatalf("GetSql error: %v", err)
	}
	t.Logf("SQL: %s", query.SQL)
	t.Logf("Params: %v", query.Params)

	t.Log("==================")
	t.Log("TestGetSql with condition")
	query, err = engine.GetSql("test.sql2", map[string]interface{}{"a": 1, "name": "test", "age": 20, "id": 1, "ids": []int{1, 2, 3}})
	if err != nil {
		t.Fatalf("GetSql error: %v", err)
	}
	t.Logf("SQL: %s", query.SQL)
	t.Logf("Params: %v", query.Params)

	t.Log("==================")
	t.Log("TestGetSql with custom function: sql6")
	query, err = engine.GetSql("test.sql6", map[string]interface{}{"ids": []int{1, 2, 3}, "trim": func(operator string, query *Query) {
		query.SQL = strings.TrimSpace(query.SQL)
		// trim operator
		query.SQL = strings.TrimPrefix(query.SQL, operator)
		query.SQL = strings.TrimSuffix(query.SQL, operator)
		query.Params = append(query.Params, "444")
	}})
	if err != nil {
		t.Fatalf("GetSql error: %v", err)
	}
	t.Logf("SQL: %s", query.SQL)
	t.Logf("Params: %v", query.Params)
}

func TestGetSql2(t *testing.T) {
	engine := New()
	bs, _ := os.ReadFile("example.md")
	err := engine.LoadMarkdown(string(bs))
	if err != nil {
		t.Fatalf("LoadMarkdown error: %v", err)
	}

	var p = Person{
		Id:   1,
		Name: "test",
		Ids:  []string{"1", "2", "3"},
	}

	query, err := engine.GetSql("test.sql8", p)
	if err != nil {
		t.Fatalf("GetSql error: %v", err)
	}
	t.Logf("SQL: %s", query.SQL)
	t.Logf("Params: %v", query.Params)
}

func TestConditionalLine(t *testing.T) {
	engine := New()
	bs, _ := os.ReadFile("example.md")
	err := engine.LoadMarkdown(string(bs))
	if err != nil {
		t.Fatalf("LoadMarkdown error: %v", err)
	}

	// 测试条件行：name 有值，age 和 id 为零值
	args := map[string]interface{}{
		"name": "test",
		"age":  0,
		"id":   0,
		"ids":  []int{1, 2},
	}

	query, err := engine.GetSql("test.sql2", args)
	if err != nil {
		t.Fatalf("GetSql error: %v", err)
	}
	t.Logf("SQL: %s", query.SQL)
	t.Logf("Params: %v", query.Params)

	// name 有值应该输出，age 和 id 为 0 不应输出
	if !strings.Contains(query.SQL, "name =") {
		t.Error("SQL should contain 'name =' when name has value")
	}
}

func TestEmbeddedStruct(t *testing.T) {
	engine := New()

	markdown := `
# test

## embedded
` + "```sql" + `
select * from users where id = @id and name = @name and base_field = @baseField
` + "```" + `
`

	err := engine.LoadMarkdown(markdown)
	if err != nil {
		t.Fatalf("LoadMarkdown error: %v", err)
	}

	// 使用嵌入结构体
	args := EmbeddedPerson{
		Base: Base{BaseField: "base_value"},
		Person: Person{
			Id:   1,
			Name: "test",
		},
	}

	query, err := engine.GetSql("test.embedded", args)
	if err != nil {
		t.Fatalf("GetSql error: %v", err)
	}

	t.Logf("SQL: %s", query.SQL)
	t.Logf("Params: %v", query.Params)

	if len(query.Params) != 3 {
		t.Errorf("expected 3 params, got %d", len(query.Params))
	}
}

func TestEmbeddedMethod(t *testing.T) {
	engine := New()

	markdown := `
# test

## embeddedMethod
` + "```sql" + `
name is @= GetName() @
base value is @= GetBaseValue() @
` + "```" + `
`

	err := engine.LoadMarkdown(markdown)
	if err != nil {
		t.Fatalf("LoadMarkdown error: %v", err)
	}

	args := EmbeddedPerson{
		Base: Base{BaseField: "hello"},
		Person: Person{
			Id:   1,
			Name: "world",
		},
	}

	query, err := engine.GetSql("test.embeddedMethod", args)
	if err != nil {
		t.Fatalf("GetSql error: %v", err)
	}

	t.Logf("SQL: %s", query.SQL)
	t.Logf("Params: %v", query.Params)

	if !strings.Contains(query.SQL, "world") {
		t.Error("SQL should contain GetName() result 'world'")
	}
	if !strings.Contains(query.SQL, "hello") {
		t.Error("SQL should contain GetBaseValue() result 'hello'")
	}
}

type Base struct {
	BaseField string
}

func (b Base) GetBaseValue() string {
	return b.BaseField
}

type Person struct {
	Id   int
	Name string

	Ids []string
}

func (p Person) GetName() string {
	return p.Name
}

func (p *Person) GetId() int {
	return p.Id
}

func (p Person) Trim(operator string, query *Query) {
	query.SQL = strings.TrimSpace(query.SQL)
	query.SQL = strings.TrimPrefix(query.SQL, operator)
	query.SQL = strings.TrimSuffix(query.SQL, operator)
	for i, v := range query.Params {
		query.Params[i] = v.(string) + " ok "
	}
	query.SQL = " " + query.SQL + " "
}

type EmbeddedPerson struct {
	Base
	Person
}

func TestPrivateField(t *testing.T) {
	engine := New()

	markdown := `
# test

## privateField
` + "```sql" + `
select * from users where private = @privateField and public = @PublicField
` + "```" + `
`

	err := engine.LoadMarkdown(markdown)
	if err != nil {
		t.Fatalf("LoadMarkdown error: %v", err)
	}

	// 使用指针传入才能访问私有字段
	args := &StructWithPrivate{
		privateField: "secret",
		PublicField:  "public",
	}

	query, err := engine.GetSql("test.privateField", args)
	if err != nil {
		t.Fatalf("GetSql error: %v", err)
	}

	t.Logf("SQL: %s", query.SQL)
	t.Logf("Params: %v", query.Params)

	if len(query.Params) != 2 {
		t.Errorf("expected 2 params, got %d", len(query.Params))
	}
}

type StructWithPrivate struct {
	privateField string
	PublicField  string
}

func TestFuncSyntax(t *testing.T) {
	engine := New()

	// 测试内置指令语法 @use path @define name @cover name
	markdown := `
# test

## useFuncSyntax
` + "```sql" + `
@use test.defineFunc {
	@cover block {
		covered content
	}
}
` + "```" + `

## defineFunc
` + "```sql" + `
before define
@define block {
	default content
}
after define
` + "```" + `
`

	err := engine.LoadMarkdown(markdown)
	if err != nil {
		t.Fatalf("LoadMarkdown error: %v", err)
	}

	query, err := engine.GetSql("test.useFuncSyntax", nil)
	if err != nil {
		t.Fatalf("GetSql error: %v", err)
	}

	t.Logf("SQL: %s", query.SQL)

	// 验证 cover 覆盖了 define
	if !strings.Contains(query.SQL, "covered content") {
		t.Error("SQL should contain covered content")
	}
	if strings.Contains(query.SQL, "default content") {
		t.Error("SQL should NOT contain default content (should be covered)")
	}
}

func TestConditionNotExist(t *testing.T) {
	engine := New()

	markdown := `
# test

## condNotExist
` + "```sql" + `
select * from users
where 1 = 1
    and name = @name?
    and notexist = @notexist?
` + "```" + `
`

	err := engine.LoadMarkdown(markdown)
	if err != nil {
		t.Fatalf("LoadMarkdown error: %v", err)
	}

	// 只传 name，不传 notexist
	args := map[string]interface{}{
		"name": "test",
	}

	query, err := engine.GetSql("test.condNotExist", args)
	if err != nil {
		t.Fatalf("GetSql error: %v", err)
	}

	t.Logf("SQL: %s", query.SQL)
	t.Logf("Params: %v", query.Params)

	// name 应该输出，notexist 行应该跳过
	if !strings.Contains(query.SQL, "name =") {
		t.Error("SQL should contain 'name ='")
	}
	if strings.Contains(query.SQL, "notexist =") {
		t.Error("SQL should NOT contain 'notexist =' when field doesn't exist")
	}
}

func TestRegisterFunc(t *testing.T) {
	engine := New()

	markdown := `
# test

## customFunc
` + "```sql" + `
result is @= CustomFunc("hello") @
` + "```" + `
`

	err := engine.LoadMarkdown(markdown)
	if err != nil {
		t.Fatalf("LoadMarkdown error: %v", err)
	}

	// 注册自定义函数
	engine.RegisterFunc("CustomFunc", func(s string) string {
		return "custom: " + s
	})

	query, err := engine.GetSql("test.customFunc", map[string]interface{}{})
	if err != nil {
		t.Fatalf("GetSql error: %v", err)
	}

	t.Logf("SQL: %s", query.SQL)

	if !strings.Contains(query.SQL, "custom: hello") {
		t.Error("SQL should contain custom function result")
	}
}

func TestNestedDefineOverride(t *testing.T) {
	engine := New()

	markdown := `
# test

## sql7_5
` + "```sql" + `
pre
===

@define abc {
    and id = @id
    and id2 = @id2

    @define d {
        this is d block
    }
}
` + "```" + `

## sql8
` + "```sql" + `
@use test.sql7_5 {
    @cover abc.d {
        d block changed
    }
}
` + "```" + `

## sql8_2
` + "```sql" + `
@use test.sql7_5 {
    @cover abc {
        abc block changed
    }
}
` + "```" + `
`

	err := engine.LoadMarkdown(markdown)
	if err != nil {
		t.Fatalf("LoadMarkdown error: %v", err)
	}

	// 测试覆盖嵌套的 define 块 abc.d
	query, err := engine.GetSql("test.sql8", map[string]interface{}{
		"id":  1,
		"id2": 2,
	})
	if err != nil {
		t.Fatalf("GetSql error: %v", err)
	}

	t.Logf("SQL8: %s", query.SQL)

	// 应该包含修改后的 d 块内容
	if !strings.Contains(query.SQL, "d block changed") {
		t.Error("SQL should contain 'd block changed' from cover")
	}
	// 原始 d 块内容应该被替换
	if strings.Contains(query.SQL, "this is d block") {
		t.Error("SQL should NOT contain original 'd block' content")
	}
	// abc 块的其他部分应该保留
	if !strings.Contains(query.SQL, "and id =") {
		t.Error("SQL should contain 'and id =' from abc block")
	}

	// 测试覆盖外层的 abc 块
	query2, err := engine.GetSql("test.sql8_2", map[string]interface{}{})
	if err != nil {
		t.Fatalf("GetSql error: %v", err)
	}

	t.Logf("SQL8_2: %s", query2.SQL)

	// 应该包含修改后的 abc 块内容
	if !strings.Contains(query2.SQL, "abc block changed") {
		t.Error("SQL should contain 'abc block changed' from cover")
	}
	// 原始 abc 块内容应该被完全替换
	if strings.Contains(query2.SQL, "and id =") {
		t.Error("SQL should NOT contain original abc block content")
	}
}

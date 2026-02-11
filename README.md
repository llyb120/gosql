# gosql

一个高性能的 **SQL 模板引擎**，支持将 SQL 模板写在 Markdown 文件中，通过变量替换、条件控制、循环等功能动态生成 SQL 语句和参数列表。

## ✨ 特性

- 🚀 **高性能**：支持静态编译模式，将模板编译为 Go 函数，接近原生性能
- 📝 **Markdown 语法**：在熟悉的 Markdown 中编写 SQL 模板，易于维护
- 🔧 **丰富的模板语法**：支持变量、条件、循环、片段复用等完整功能
- 🛡️ **安全参数化**：自动处理 SQL 注入防护，生成带占位符的安全 SQL
- 🎯 **灵活的参数传递**：支持 map、结构体、结构体方法等多种参数形式
- 📦 **零依赖**：仅依赖标准库和 goscript2，轻量级集成

## 📦 安装

```bash
go get github.com/llyb120/gosql
```

## 🚀 快速开始

### 1️⃣ 创建 Markdown 模板文件

创建 `queries.md`：

````md
# user

## findById
根据用户ID查询用户信息
```sql
select * from users 
where id = @id
  and status = @status?
```

## findByCondition
根据条件查询用户列表
```sql
select * from users
where 1 = 1
  and name like @name?
  and age >= @minAge?
  and status in (@statuses)
order by create_time desc
limit @limit
```

## createUser
创建用户
```sql
insert into users (name, email, age, status)
values (@name, @email, @age, @status)
```
````

### 2️⃣ 在 Go 代码中使用

```go
package main

import (
    "database/sql"
    "fmt"
    "log"

    _ "github.com/go-sql-driver/mysql"
    "github.com/llyb120/gosql"
)

func main() {
    // 初始化引擎
    engine := gosql.New()
    
    // 加载模板文件
    content, err := os.ReadFile("queries.md")
    if err != nil {
        log.Fatal(err)
    }
    if err := engine.LoadMarkdown(string(content)); err != nil {
        log.Fatal(err)
    }

    db, _ := sql.Open("mysql", "user:pass@tcp(localhost:3306)/db")
    defer db.Close()

    // 示例1: 查询单个用户
    query, err := engine.GetSql("user.findById", map[string]interface{}{
        "id": 123,
        "status": "active",
    })
    if err != nil {
        log.Fatal(err)
    }
    
    fmt.Println("SQL:", query.SQL)
    fmt.Println("Params:", query.Params)
    // 输出: SQL: select * from users where id = ? and status = ?
    //      Params: [123 active]

    // 执行查询
    var name string
    err = db.QueryRow(query.SQL, query.Params...).Scan(&name)
    if err != nil {
        log.Fatal(err)
    }

    // 示例2: 条件查询（数组参数自动展开）
    query, err = engine.GetSql("user.findByCondition", map[string]interface{}{
        "name":    "张%",
        "minAge":  18,
        "statuses": []string{"active", "pending"},
        "limit":   10,
    })
    
    rows, err := db.Query(query.SQL, query.Params...)
    if err != nil {
        log.Fatal(err)
    }
    defer rows.Close()
    
    // 处理结果...
}
```

### 3️⃣ 使用结构体传参

```go
type UserQuery struct {
    ID     int    `db:"id"`
    Name   string `db:"name"`
    Status string `db:"status"`
}

// 结构体字段会自动映射为模板变量
query, err := engine.GetSql("user.findById", UserQuery{
    ID: 123,
    Status: "active",
})
```

## 📝 模板语法详解

### 🏗️ Markdown 组织结构

gosql 使用标准的 Markdown 语法组织 SQL 模板：

- `# 命名空间` - 一级标题作为命名空间，用于逻辑分组
- `## 模板名` - 二级标题作为模板名称
- ` ```sql ... ``` ` - SQL 代码块，只有标记为 `sql` 的代码块才会被解析

**示例结构：**
````md
# user  <!-- 命名空间 -->

## findById  <!-- 模板名 -->
查询用户信息
```sql  <!-- SQL 代码块 -->
select * from users where id = @id
```

## updateStatus  <!-- 另一个模板 -->
更新用户状态
```sql
update users set status = @status where id = @id
```
````

**模板路径格式：**
- `namespace.name` - 执行整个模板
- `namespace.name.define` - 只执行模板中的特定 `@define` 块

### 🎯 参数传递方式

#### 1. Map 参数（最常用）
```go
engine.GetSql("user.findById", map[string]interface{}{
    "id": 123,
    "status": "active",
    "tags": []string{"vip", "premium"},
})
```

#### 2. 结构体参数
```go
type UserQuery struct {
    ID     int      `json:"id"`
    Status string   `json:"status"`
    Tags   []string `json:"tags"`
}

query := UserQuery{ID: 123, Status: "active", Tags: []string{"vip"}}
engine.GetSql("user.findById", query)
```

#### 3. 结构体方法绑定
```go
type UserService struct {
    db *sql.DB
}

func (s *UserService) GetTableName() string {
    return "users"
}

func (s *UserService) IsActive(status string) bool {
    return status == "active"
}

// 方法会在模板中自动可用
engine.GetSql("user.list", &UserService{db: db})
```

### 🔧 核心模板语法

#### 1️⃣ 变量占位：`@var`

最基础的变量替换，自动参数化：

```sql
-- 单个值
where id = @id

-- 数组/切片自动展开
where id in (@ids)

-- 在表达式中使用
and name = @name
```

**参数处理：**
- 单个值 → 生成 `?`，添加一个参数
- 数组/切片 → 生成 `?, ?, ?`，添加多个参数
- nil/空值 → 生成 `NULL`

**复杂表达式：**
当变量名包含空格或其他词法器无法正确识别的字符时，使用 `@{expr}` 形式：

```sql
-- 复杂表达式（词法器无法识别时使用）
@{user name} = @value
```

#### 2️⃣ 原样输出：`@=expr`

用于表名、列名等不能参数化的场景：

```sql
-- 动态表名
select * from @=tableName where id = @id

-- 动态列名  
order by @=sortColumn @=sortDirection

-- 函数调用
and @=GetStatusCondition()
```

**复杂表达式：**
当表达式包含空格或其他词法器无法正确识别的字符时，使用 `@={}` 形式：

```sql
-- 复杂表达式（词法器无法识别时使用）
name is @={GetName()}
id is @={GetId()}
```

⚠️ **安全警告**：原样输出不会进行参数化，请确保输入值的安全性，避免 SQL 注入。

#### 3️⃣ 条件行：`@var?`

当变量为空值时，整行会被跳过：

```sql
where 1 = 1
  and name = @name?      -- name 为空时此行消失
  and age = @age?        -- age 为 0 时此行消失
  and status = @status?  -- status 为 "" 时此行消失
```

**空值判断规则：**
- `string`: 空字符串 `""`
- `slice`: 长度为 0
- `int/float`: 0
- `bool`: false
- `pointer`: nil
- `interface{}`: nil

#### 4️⃣ 条件分支：`@if / @else if / @else`

完整的条件控制：

```sql
@if id > 0 {
    and id = @id
} else if name != "" {
    and name like @name
} else {
    and status = 'default'
}
```

**条件表达式：**
- 支持所有 Go 比较操作符：`>`, `<`, `>=`, `<=`, `==`, `!=`
- 支持逻辑操作符：`&&`, `||`, `!`
- 支持函数调用：`@if IsActive(status) { ... }`

#### 5️⃣ 循环：`@for`

**传统 for 循环：**
```sql
@for i := 0; i < 3; i++ {
    and col@=i = @i
}
```

**range 循环：**
```sql
-- 只需要值
@for _, id := range ids {
    or id = @id
}

-- 需要索引和值
@for i, id := range ids {
    or id_@=i = @id
}
```

**遍历map：**
```sql
@for key, value := range conditions {
    and @=key = @value
}
```

#### 6️⃣ 片段定义与复用：`@define / @use / @cover`

用于复杂 SQL 的模块化管理：

**定义片段：**
```sql
@define baseConditions {
    and status = @status
    and create_time >= @startTime
}

@define userConditions {
    @define baseConditions {  -- 嵌套定义
        and status = @status
    }
    and age >= @minAge
}
```

**使用其他模板：**
```sql
-- 复用 user.selectBase 的完整内容
@use user.selectBase {
    -- 可以覆盖其中的 define 块
    @cover baseConditions {
        and status = 'active'  -- 覆盖条件
        and priority = @priority
    }
}
```

**覆盖嵌套片段：**
```sql
@use user.complexQuery {
    @cover outer.inner {  -- 使用点号路径
        and new_field = @value
    }
}
```

#### 7️⃣ 自定义函数块

类似 `Trim` 的代码块处理函数：

```sql
-- 使用内置 Trim 函数去掉多余的 "and"
select * from users
where
@Trim("and") {
    @for _, status := range statuses {
        and status = @status
    }
}
```

**注册自定义函数：**
```go
engine.RegisterFunc("JoinWith", func(separator string, query *gosql.Query) {
    // 自定义处理逻辑
    query.SQL = strings.Join(strings.Split(query.SQL, " "), separator)
})
```

**在模板中使用：**
```sql
@JoinWith(",") {
    @for _, col := range columns {
        @=col
    }
}
```


## 🚀 高级功能

### ⚡ 性能优化与执行模式

gosql 支持三种执行模式，可根据需求选择：

#### 1. 自动模式（默认）- `ExecModeAuto`
```go
engine := gosql.New()  // 默认为自动模式
```
- 优先使用静态编译函数（高性能）
- 回退到动态解释执行（灵活性）
- 适合大多数场景

#### 2. 静态模式 - `ExecModeStatic`
```go
engine.SetMode(gosql.ExecModeStatic)
```
- 仅使用预编译的静态函数
- 最高性能，接近原生 Go 代码
- 需要配合代码生成工具使用

#### 3. 动态模式 - `ExecModeDynamic`
```go
engine.SetMode(gosql.ExecModeDynamic)
```
- 始终使用 AST 解释执行
- 最高灵活性，支持运行时修改
- 性能相对较低

### 🛠️ 代码生成工具

为了获得最佳性能，可以使用 `gosqlgen` 工具将 SQL 模板编译为静态 Go 函数：

#### 安装工具
```bash
go install github.com/llyb120/gosql/cmd/gosqlgen@latest
```

#### 基本用法
```bash
# 生成静态代码
gosqlgen -input queries.md -output queries_gen.go -package myapp

# 或在 Go 文件中使用 go generate
//go:generate go run github.com/llyb120/gosql/cmd/gosqlgen -input queries.md -package myapp
```

#### 高级选项
```bash
gosqlgen -input queries.md \
         -output queries_gen.go \
         -package myapp \
         -tag gosql_static \
         -self  # 在 gosql 包内部生成
```

生成的代码示例：
```go
// 自动生成的静态函数
func static_user_findById_12345(ctx *gosql.StaticContext) error {
    ctx.WriteSQL("select * from users where id = ")
    ctx.AddParam(ctx.Get("id"))
    if ctx.Has("status") {
        ctx.WriteSQL(" and status = ")
        ctx.AddParam(ctx.Get("status"))
    }
    return nil
}

func init() {
    gosql.RegisterStaticFunc("user.findById", static_user_findById_12345)
}
```

### 📋 完整 API 参考

#### Engine 核心方法

```go
// 创建新引擎
func New() *Engine

// 加载 Markdown 内容（字符串）
func (e *Engine) LoadMarkdown(content string) error

// 加载 Markdown 文件（需要先读取文件内容）
// content, _ := os.ReadFile("filename.md")
// engine.LoadMarkdown(string(content))

// 获取渲染后的 SQL
func (e *Engine) GetSql(path string, args interface{}) (Query, error)

// 注册自定义函数
func (e *Engine) RegisterFunc(name string, fn interface{})

// 设置执行模式
func (e *Engine) SetMode(mode ExecMode)

// 获取执行模式
func (e *Engine) GetMode() ExecMode
```

#### 全局便捷函数

```go
// 初始化默认引擎
func Init() *Engine

// 加载内容到默认引擎
func Load(content string) error

// 从默认引擎获取 SQL
func GetSqlFromDefault(path string, args interface{}) (Query, error)
```

#### Query 结构体

```go
type Query struct {
    SQL    string        // 生成的 SQL 语句
    Params []interface{} // 参数列表
}
```

### 🔧 自定义函数详解

#### 1. 普通函数注册
```go
// 注册简单函数
engine.RegisterFunc("ToUpper", func(s string) string {
    return strings.ToUpper(s)
})

// 注册多参数函数
engine.RegisterFunc("InRange", func(val, min, max int) bool {
    return val >= min && val <= max
})
```

#### 2. 查询处理函数
```go
// 注册查询处理函数（类似 Trim）
engine.RegisterFunc("ProcessQuery", func(prefix string, query *Query) {
    query.SQL = prefix + query.SQL
    // 可以修改 query.Params
})
```

#### 3. 结构体方法自动绑定
```go
type UserService struct {
    db *sql.DB
}

func (s *UserService) GetTableName() string {
    return "users"
}

func (s *UserService) FilterByStatus(status string) string {
    return fmt.Sprintf("status = '%s'", status)
}

// 传入结构体实例，方法自动可用
engine.GetSql("user.list", &UserService{db: db})
```

### 🎯 实战示例

#### 示例1：动态查询构建
````md
# report

## dynamicQuery
动态报表查询
```sql
select 
    id,
    name,
    @if includeEmail { email, }
    @if includePhone { phone, }
    create_time
from users
where 1 = 1
  and status = @status?
  @if minAge > 0 {
    and age >= @minAge
  }
  @if len(tags) > 0 {
    and tags in (@tags)
  }
order by @=sortColumn @=sortDirection
limit @limit
offset @offset
```
````

```go
// 使用示例
query, err := engine.GetSql("report.dynamicQuery", map[string]interface{}{
    "includeEmail": true,
    "includePhone": false,
    "status": "active",
    "minAge": 18,
    "tags": []string{"vip", "premium"},
    "sortColumn": "create_time",
    "sortDirection": "desc",
    "limit": 20,
    "offset": 0,
})
```

#### 示例2：复杂查询复用
````md
# common

## baseUserQuery
基础用户查询
```sql
select u.id, u.name, u.email, u.status
from users u
@define baseConditions {
    and u.status = @status
    and u.create_time >= @startTime
}
```

# user

## activeUsers
活跃用户查询
```sql
@use common.baseUserQuery {
    @cover baseConditions {
        and u.status = 'active'
        and u.last_login_time >= @startTime
    }
}
where u.login_count > @minLoginCount
```
````

### ⚠️ 最佳实践与注意事项

#### 安全性
- ✅ **推荐**：使用 `@var` 进行参数化查询
- ⚠️ **谨慎**：使用 `@=expr` 时务必验证输入安全性
- 🚫 **禁止**：直接拼接用户输入到 SQL 中

#### 性能优化
- 🚀 使用代码生成工具获得最佳性能
- 📊 预编译模板，避免运行时解析开销
- 🔄 复用 Engine 实例，避免重复创建

#### 代码组织
- 📁 按功能模块组织命名空间
- 🏷️ 使用有意义的模板名称
- 📝 在模板中添加注释说明

#### 错误处理
```go
query, err := engine.GetSql("user.findById", params)
if err != nil {
    if err == gosql.ERR_TEMPLATE_NOT_FOUND {
        // 模板不存在
    } else {
        // 其他错误（语法错误、参数错误等）
    }
}
```

### 🔍 调试技巧

#### 1. 启用详细错误信息
```go
// 错误信息会包含具体的语法错误位置和上下文
query, err := engine.GetSql("user.query", params)
if err != nil {
    fmt.Printf("Error: %v\n", err)
    // 输出示例：template user.query: line 5: unexpected token '@'
}
```

#### 2. 查看生成的 SQL
```go
query, err := engine.GetSql("user.list", params)
if err == nil {
    fmt.Printf("SQL: %s\n", query.SQL)
    fmt.Printf("Params: %v\n", query.Params)
}
```

#### 3. 测试单个模板
```go
// 使用测试数据验证模板逻辑
testData := map[string]interface{}{
    "id": 123,
    "status": "active",
    "tags": []string{"vip"},
}
query, err := engine.GetSql("user.findById", testData)
```

## 📚 更多资源

- **完整示例**：查看项目中的 `example.md` 和 `example_gen.go`
- **测试用例**：参考 `gosql_test.go` 了解更多用法
- **性能对比**：静态编译模式比动态模式快 5-10 倍

## 🤝 贡献

欢迎提交 Issue 和 Pull Request！

## 📄 许可证

MIT License

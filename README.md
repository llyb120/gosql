# gosql - Go SQL 模板引擎

基于 goscript2 的 SQL 模板引擎，支持动态 SQL 生成。

## 特性

- ✅ 使用 Markdown 文件管理 SQL 模板
- ✅ 支持变量绑定（自动参数化）
- ✅ 支持流程控制（if/else if/else/for）
- ✅ 支持模板复用（use/define/cover）
- ✅ 支持数组自动展开
- ✅ 详细的错误信息提示

## 安装

```bash
go get github.com/llyb120/gosql
```

## 快速开始

### 1. 创建 SQL 模板文件（Markdown 格式）

```markdown
# user

## findById
根据 ID 查询用户
` + "```sql" + `
select * from users 
where id = @id
` + "```" + `

## findByCondition
条件查询
` + "```sql" + `
select * from users
where 1 = 1
@if name != "" {
    and name = @name
}
@if age > 0 {
    and age = @age
}
` + "```" + `
```

### 2. 使用模板引擎

```go
package main

import (
    "fmt"
    "github.com/llyb120/gosql"
)

func main() {
    // 创建引擎
    engine := gosql.New()

    // 加载 Markdown 模板
    markdown := `
# user

## findById
` + "`" + "`" + "`" + `sql
select * from users where id = @id
` + "`" + "`" + "`" + `
`
    err := engine.LoadMarkdown(markdown)
    if err != nil {
        panic(err)
    }

    // 使用 map 作为参数
    sql, params, err := engine.GetSql("user.findById", map[string]interface{}{
        "id": 1,
    })

    fmt.Println("SQL:", sql)
    fmt.Println("Params:", params)
    // 输出:
    // SQL: select * from users where id = ?
    // Params: [1]

    // 也可以使用结构体作为参数
    type Args struct {
        Id   int
        Name string
    }
    sql, params, err = engine.GetSql("user.findById", Args{Id: 1})
}
```

## 模板语法

### 变量绑定

使用 `@变量名` 输出占位符 `?` 并添加参数：

```sql
select * from users where id = @id and name = @name
```

数组会自动展开：

```sql
-- 如果 ids = [1, 2, 3]
select * from users where id in (@ids)
-- 生成: select * from users where id in (?, ?, ?)
-- 参数: [1, 2, 3]
```

### 直接输出

使用 `@=变量名` 或 `@=变量名@` 直接输出值（不参数化）：

```sql
select * from @=tableName@ where id = @id
-- 如果 tableName = "users", id = 1
-- 生成: select * from users where id = ?
-- 参数: [1]
```

### 流程控制

**if/else if/else:**

```sql
select * from users
where 1 = 1
@if status > 0 {
    and status = @status
} else if status < 0 {
    and status is null
} else {
    and status = 0
}
```

**for 循环:**

```sql
-- 传统 for 循环
@for i := 0; i < 3; i++ {
    union select @i
}

-- range 形式
@for i, v := range items {
    union select @v
}
```

### 模板复用

**define - 定义可覆盖的代码块:**

```sql
select * from users
where 1 = 1
@define conditions {
    and status = 1
}
```

**use - 引用其他模板:**

```sql
-- 引用 user.base 模板
@use user.base {
    -- 覆盖 conditions 块
    @cover conditions {
        and status = 0
        and deleted = 0
    }
}
```

**引用特定 define 块:**

```sql
-- 只引用 user.base 中的 conditions 块
@use user.base.conditions {
}
```

### 嵌套 define

```sql
@define outer {
    where 1 = 1
    @define inner {
        and id = @id
    }
}
```

## API

### Engine

```go
// 创建引擎
engine := gosql.New()

// 加载 Markdown 模板
err := engine.LoadMarkdown(markdownContent)

// 获取 SQL
// path: "namespace.name" 或 "namespace.name.define"
// args: map[string]interface{} 或 struct
sql, params, err := engine.GetSql(path, args)
```

### 全局函数

```go
// 初始化默认引擎
gosql.Init()

// 加载模板到默认引擎
gosql.Load(markdownContent)

// 从默认引擎获取 SQL
sql, params, err := gosql.GetSqlFromDefault("user.findById", args)
```

## 错误处理

解析错误会包含详细的上下文信息：

```
template test.sql: line 5: expected '{' after if condition
>>> @if a > 0
        and name = @name
```

## 许可证

MIT License


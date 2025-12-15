# gosql

一个轻量级的 **SQL 模板引擎**。

SQL 写在 Markdown 的 `sql` 代码块里，然后在 SQL 里用 `@变量`、`@if/@for`、`@define/@use/@cover` 做拼装与复用。

渲染后得到：

- **SQL**：带 `?` 占位符的 SQL 字符串
- **Params**：按顺序收集的参数列表（可直接给 `database/sql`、gorm Raw 等使用）

简单说就是：**Markdown 里写模板，Go 里传参数，最后拿到 SQL + Params。**

## 安装

```bash
go get github.com/llyb120/gosql
```

## 1 分钟上手

### 1) 写一个 Markdown 模板

准备 `example.md`：

````md
# test

## sql1
```sql
select * from users
where id = @id
  and status in (@status)
```
````

这段模板里：

- `@id` 变成一个 `?`，并把 `id` 的值放进参数列表
- `@status` 如果是切片，会展开成 `?, ?, ?`，并把每个元素依次放进参数列表

### 2) 在 Go 里加载并渲染

```go
package main

import (
	"fmt"
	"os"

	"github.com/llyb120/gosql"
)

func main() {
	bs, _ := os.ReadFile("example.md")
	engine := gosql.New()
	if err := engine.LoadMarkdown(string(bs)); err != nil {
		panic(err)
	}

	q, err := engine.GetSql("test.sql1", map[string]interface{}{
		"id":     1,
		"status": []int{1, 2, 3},
	})
	if err != nil {
		panic(err)
	}

	fmt.Println(q.SQL)
	fmt.Println(q.Params)
}
```

## Markdown 怎么组织

规则很简单：

- `# xxx` 是 **命名空间**（namespace）
- `## yyy` 是 **模板名**（template name）
- 模板内容写在 **`sql` 代码块**里（```sql ... ```）

最终渲染使用一个 `path` 来定位模板：

- `namespace.name`：执行整个模板
- `namespace.name.define`：只执行模板里的某个 `@define` 块

例子：

- `engine.GetSql("test.sql1", args)`
- `engine.GetSql("test.sql4.a", args)`（只输出 `sql4` 中 `@define a { ... }` 的内容）

### `sql` 代码块怎么写

gosql 只会解析 **language 标记为 `sql` 的 fenced code block**。

````md
## sql1
```sql
select * from users
where id = @id
```
````

注意：

- 代码块必须写 `sql`（```sql），否则不会被当作模板
- 代码块必须正确闭合（结尾也是 ```）

## 参数怎么传（args）

`args` 支持：

- `map[string]interface{}`：键就是变量名
- 结构体 / 结构体指针：字段会展开成变量（同时支持 `Id` 和 `id`）
- 结构体方法：会绑定到模板执行环境中，可在表达式里调用（值/指针接收器都支持）
- 私有字段：需要传入 **指针** 才能读取（内部使用了 `unsafe`）

## 模板语法（从最常用开始）

### 1) 参数占位：`@var`

说明：

- `@id` 输出 `?`，并把 `id` 的值追加到 `Params`
- 如果值是 slice/array：输出 `?, ?, ?` 并把每个元素依次追加到 `Params`

```sql
and id = @id
and id in (@ids)
```

### 2) 原样输出（不参数化）：`@=expr@`

用于表名、列名、片段等 **不能参数化** 的位置。

```sql
select * from @=tableName@ where id = @id
```

这里不会走参数化：**请自行确保安全，避免 SQL 注入**。

### 3) 条件行：`@name?`

写在一整行里：当变量不存在或为零值时，这一行会被跳过。

```sql
where 1 = 1
  and name = @name?
  and age = @age?
```

### 4) 条件分支：`@if / else if / else`

```sql
@if a > 0 {
    and name = @name
} else if a < 0 {
    and age = @age
} else {
    and id = @id
}
```

条件表达式使用 goscript2 执行，变量来自 `args` 展开后的 scope。

### 5) 循环：`@for`

传统 for：

```sql
@for i := 0; i < 3; i++ {
    and col@=i@ = @i
}
```

range：

```sql
@for i, v := range ids {
    and id = @v
}
```

### 6) 片段定义与复用：`@define / @use / @cover`

用来把复杂 SQL 拆成可复用片段。

定义片段：

```sql
@define a {
    and id = @id
}
```

复用其它模板并覆盖片段：

```sql
@use test.sql4 {
    @cover a {
        and id <> @id
    }
}
```

覆盖嵌套片段（用点号指定路径）：

```sql
@cover abc.d {
    d block changed
}
```

多级 `cover` 一般会配合 `use` 用（从别的模板里复用一套 define，然后按需改里面的某几块）：

```sql
@use test.sql7_5 {
    @cover abc {
        abc changed
    }

    @cover abc.d {
        d block changed
    }
}
```

### 7) 代码块函数（类似 `Trim`）

有时候你会希望“包一层块”，让引擎对块里的内容做一点处理（比如把循环里每行都以 `and` 开头的条件，最后自动去掉多余的 `and`）。

用法：

```sql
select * from table
where
@Trim("and") {
    @for _, v := range ids {
        and id = @v
    }
}
```

```go
func Trim(operator string, query *Query) {
	query.SQL = strings.TrimSpace(query.SQL)
	query.SQL = strings.TrimPrefix(query.SQL, operator)
	query.SQL = strings.TrimSuffix(query.SQL, operator)
	for i, v := range query.Params {
		query.Params[i] = v.(string) + " ok "
	}
	query.SQL = " " + query.SQL + " "
}
```


## 核心 API

- `gosql.New() *Engine`：创建引擎实例
- `(*Engine).LoadMarkdown(content string) error`：加载 markdown 内容（会预编译模板）
- `(*Engine).GetSql(path string, args interface{}) (Query, error)`：渲染并返回 `{SQL, Params}`
- `(*Engine).RegisterFunc(name string, fn interface{})`：注册自定义函数（模板内可调用）

也提供默认引擎的便捷函数：

- `gosql.Init() *Engine`
- `gosql.Load(content string) error`
- `gosql.GetSqlFromDefault(path string, args interface{}) (Query, error)`

## 自定义函数

可以在 Go 侧注册函数，然后在模板表达式里调用：

```go
engine.RegisterFunc("CustomFunc", func(s string) string {
	return "custom: " + s
})
```

```sql
result is @= CustomFunc("hello") @
```

如果你传入的是结构体，它的方法也会自动绑定，可在模板里直接调用（例如 `@= GetName() @`）。

## 常见注意事项

- `@=...@` 不会参数化：用于动态片段时请自行保证安全
- 条件行 `@x?`：当 `x` 不存在或为零值会跳过整行；如果你希望 `0` 也输出，请不要用 `?`
- 私有字段读取：要传结构体指针，否则无法读取
- `path` 格式：必须至少包含 `namespace.name`，否则会报 `invalid path`

## 更多示例

仓库内的 `example.md` 包含较完整的语法覆盖（if/for/use/define/trim/slot 等）。

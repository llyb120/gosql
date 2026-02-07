// gosqlgen 是 gosql 的代码生成工具
// 用于将 SQL 模板 Markdown 文件编译为静态 Go 函数，提升线上运行性能
//
// 用法:
//
//	gosqlgen -input <file.md> [-output <file.go>] [-package <name>]
//
// 也可以配合 go generate 使用:
//
//	//go:generate go run github.com/llyb120/gosql/cmd/gosqlgen -input queries.md -package mypackage
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/llyb120/gosql"
)

func main() {
	input := flag.String("input", "", "输入的 Markdown 模板文件路径")
	output := flag.String("output", "", "输出的 Go 文件路径（默认: <input>_gosql_gen.go）")
	pkg := flag.String("package", "", "包名（默认: 从输出目录自动检测）")
	self := flag.Bool("self", false, "在 gosql 包内部生成（不添加 import 和包前缀）")
	tag := flag.String("tag", "", "添加 //go:build 标签（如 gosql_static）")
	flag.Parse()

	if *input == "" {
		fmt.Fprintln(os.Stderr, "用法: gosqlgen -input <file.md> [-output <file.go>] [-package <name>]")
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintln(os.Stderr, "选项:")
		flag.PrintDefaults()
		os.Exit(1)
	}

	// 读取输入文件
	content, err := os.ReadFile(*input)
	if err != nil {
		fmt.Fprintf(os.Stderr, "读取文件失败 %s: %v\n", *input, err)
		os.Exit(1)
	}

	// 确定输出路径
	outputPath := *output
	if outputPath == "" {
		ext := filepath.Ext(*input)
		base := strings.TrimSuffix(*input, ext)
		outputPath = base + "_gosql_gen.go"
	}

	// 确定包名
	packageName := *pkg
	if packageName == "" {
		// 自动从输出目录检测
		dir, err := filepath.Abs(filepath.Dir(outputPath))
		if err != nil {
			dir = filepath.Dir(outputPath)
		}
		packageName = filepath.Base(dir)
		if packageName == "." || packageName == "" {
			packageName = "main"
		}
	}

	// 解析 Markdown 模板
	templates, err := gosql.ParseMarkdown(string(content))
	if err != nil {
		fmt.Fprintf(os.Stderr, "解析 Markdown 失败: %v\n", err)
		os.Exit(1)
	}

	if len(templates) == 0 {
		fmt.Fprintf(os.Stderr, "警告: 没有在 %s 中找到任何 SQL 模板\n", *input)
		os.Exit(0)
	}

	// 将模板解析为 AST
	asts := make(map[string]*gosql.TemplateAST)
	for _, tmpl := range templates {
		key := tmpl.Namespace + "." + tmpl.Name
		ast, err := gosql.ParseTemplate(tmpl.Content)
		if err != nil {
			fmt.Fprintf(os.Stderr, "解析模板 %s 失败: %v\n", key, err)
			os.Exit(1)
		}
		ast.Namespace = tmpl.Namespace
		ast.Name = tmpl.Name
		asts[key] = ast
	}

	// 生成代码
	// -self 模式: pkgPrefix="" 表示在 gosql 包内部生成（无 import、无前缀）
	// 默认模式: pkgPrefix="gosql." 表示在外部包中使用（需要 import gosql）
	pkgPrefix := "gosql."
	if *self {
		pkgPrefix = ""
	}
	gen := gosql.NewCodeGenerator(pkgPrefix)
	code, err := gen.GenerateFile(templates, asts, packageName, filepath.Base(*input), *tag)
	if err != nil {
		fmt.Fprintf(os.Stderr, "代码生成失败: %v\n", err)
		os.Exit(1)
	}

	// 确保输出目录存在
	outputDir := filepath.Dir(outputPath)
	if outputDir != "." && outputDir != "" {
		if err := os.MkdirAll(outputDir, 0755); err != nil {
			fmt.Fprintf(os.Stderr, "创建目录失败 %s: %v\n", outputDir, err)
			os.Exit(1)
		}
	}

	// 写入输出文件
	if err := os.WriteFile(outputPath, []byte(code), 0644); err != nil {
		fmt.Fprintf(os.Stderr, "写入文件失败 %s: %v\n", outputPath, err)
		os.Exit(1)
	}

	fmt.Printf("gosqlgen: %s -> %s (%d 个模板)\n", *input, outputPath, len(templates))
}

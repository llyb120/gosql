package gosql

import (
	"bufio"
	"fmt"
	"strings"
)

// SQLTemplate 表示一个 SQL 模板
type SQLTemplate struct {
	Namespace   string                  // 一级标题（命名空间）
	Name        string                  // 二级标题（SQL 名称）
	Description string                  // SQL 描述
	Content     string                  // SQL 模板内容
	Defines     map[string]*DefineBlock // define 块
}

// DefineBlock 表示一个 define 代码块
type DefineBlock struct {
	Name    string
	Content string
	Defines map[string]*DefineBlock // 嵌套的 define
}

// TemplateStore 存储所有解析的模板
type TemplateStore struct {
	templates map[string]*SQLTemplate // key: namespace.name
}

// NewTemplateStore 创建模板存储
func NewTemplateStore() *TemplateStore {
	return &TemplateStore{
		templates: make(map[string]*SQLTemplate),
	}
}

// Get 获取模板
func (ts *TemplateStore) Get(key string) (*SQLTemplate, bool) {
	t, ok := ts.templates[key]
	return t, ok
}

// Set 设置模板
func (ts *TemplateStore) Set(key string, t *SQLTemplate) {
	ts.templates[key] = t
}

// ParseMarkdown 解析 markdown 文件内容，提取 SQL 模板
func ParseMarkdown(content string) ([]*SQLTemplate, error) {
	var templates []*SQLTemplate
	scanner := bufio.NewScanner(strings.NewReader(content))

	var currentNamespace string
	var currentName string
	var currentDesc strings.Builder
	var sqlContent strings.Builder
	var inSQLBlock bool
	var lineNum int

	for scanner.Scan() {
		lineNum++
		line := scanner.Text()

		// 检测一级标题（命名空间）
		if strings.HasPrefix(line, "# ") && !strings.HasPrefix(line, "## ") {
			currentNamespace = strings.TrimSpace(strings.TrimPrefix(line, "# "))
			currentName = ""
			currentDesc.Reset()
			continue
		}

		// 检测二级标题（SQL 名称）
		if strings.HasPrefix(line, "## ") {
			// 保存之前的 SQL 模板（如果有）
			if currentName != "" && sqlContent.Len() > 0 {
				templates = append(templates, &SQLTemplate{
					Namespace:   currentNamespace,
					Name:        currentName,
					Description: strings.TrimSpace(currentDesc.String()),
					Content:     strings.TrimSpace(sqlContent.String()),
					Defines:     make(map[string]*DefineBlock),
				})
			}

			currentName = strings.TrimSpace(strings.TrimPrefix(line, "## "))
			currentDesc.Reset()
			sqlContent.Reset()
			inSQLBlock = false
			continue
		}

		// 检测 SQL 代码块开始
		if strings.HasPrefix(strings.TrimSpace(line), "```sql") {
			if currentNamespace == "" {
				return nil, fmt.Errorf("line %d: SQL block found without namespace (missing # heading)", lineNum)
			}
			if currentName == "" {
				return nil, fmt.Errorf("line %d: SQL block found without name (missing ## heading)", lineNum)
			}
			inSQLBlock = true
			continue
		}

		// 检测代码块结束
		if strings.TrimSpace(line) == "```" && inSQLBlock {
			inSQLBlock = false
			continue
		}

		// 收集 SQL 内容
		if inSQLBlock {
			if sqlContent.Len() > 0 {
				sqlContent.WriteString("\n")
			}
			sqlContent.WriteString(line)
		} else if currentName != "" && !inSQLBlock {
			// 收集描述（在二级标题之后、SQL块之前）
			if sqlContent.Len() == 0 {
				if currentDesc.Len() > 0 {
					currentDesc.WriteString("\n")
				}
				currentDesc.WriteString(line)
			}
		}
	}

	// 保存最后一个 SQL 模板
	if currentName != "" && sqlContent.Len() > 0 {
		templates = append(templates, &SQLTemplate{
			Namespace:   currentNamespace,
			Name:        currentName,
			Description: strings.TrimSpace(currentDesc.String()),
			Content:     strings.TrimSpace(sqlContent.String()),
			Defines:     make(map[string]*DefineBlock),
		})
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan error: %w", err)
	}

	return templates, nil
}

// LoadMarkdown 加载 markdown 内容到模板存储
func (ts *TemplateStore) LoadMarkdown(content string) error {
	templates, err := ParseMarkdown(content)
	if err != nil {
		return err
	}

	for _, t := range templates {
		key := t.Namespace + "." + t.Name
		ts.templates[key] = t
	}

	return nil
}

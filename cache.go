package gosql

import (
	"reflect"
	"strings"
	"sync"
)

// TypeCache 类型反射缓存
type TypeCache struct {
	mu     sync.RWMutex
	cache  map[reflect.Type]*CachedTypeInfo
}

// CachedTypeInfo 缓存的类型信息
type CachedTypeInfo struct {
	Type       reflect.Type
	Fields     map[string]FieldInfo     // 字段名 -> 字段信息（包括小写和原始名）
	Methods    map[string]MethodInfo    // 方法名 -> 方法信息
	PtrMethods map[string]MethodInfo    // 指针方法名 -> 方法信息
}

// FieldInfo 字段信息
type FieldInfo struct {
	Index    int
	Name     string
	Type     reflect.Type
	Embedded bool // 是否是嵌入字段
}

// MethodInfo 方法信息
type MethodInfo struct {
	Index int
	Name  string
	Type  reflect.Type
}

// 全局类型缓存
var globalTypeCache = &TypeCache{
	cache: make(map[reflect.Type]*CachedTypeInfo),
}

// GetTypeInfo 获取类型信息（带缓存）
func GetTypeInfo(t reflect.Type) *CachedTypeInfo {
	// 如果是指针，获取底层类型
	origType := t
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}

	// 先尝试读取缓存
	globalTypeCache.mu.RLock()
	info, ok := globalTypeCache.cache[t]
	globalTypeCache.mu.RUnlock()

	if ok {
		return info
	}

	// 缓存不存在，需要创建
	globalTypeCache.mu.Lock()
	defer globalTypeCache.mu.Unlock()

	// 双重检查
	if info, ok = globalTypeCache.cache[t]; ok {
		return info
	}

	info = buildTypeInfo(t, origType)
	globalTypeCache.cache[t] = info
	return info
}

// buildTypeInfo 构建类型信息
func buildTypeInfo(t reflect.Type, origType reflect.Type) *CachedTypeInfo {
	info := &CachedTypeInfo{
		Type:       t,
		Fields:     make(map[string]FieldInfo),
		Methods:    make(map[string]MethodInfo),
		PtrMethods: make(map[string]MethodInfo),
	}

	if t.Kind() != reflect.Struct {
		return info
	}

	// 递归收集字段（包括嵌入字段）
	collectFields(t, info.Fields, nil)

	// 收集值接收器方法
	for i := 0; i < t.NumMethod(); i++ {
		method := t.Method(i)
		if method.IsExported() {
			info.Methods[method.Name] = MethodInfo{
				Index: i,
				Name:  method.Name,
				Type:  method.Type,
			}
		}
	}

	// 收集指针接收器方法
	ptrType := reflect.PtrTo(t)
	for i := 0; i < ptrType.NumMethod(); i++ {
		method := ptrType.Method(i)
		if method.IsExported() {
			// 只添加值类型没有的方法
			if _, exists := info.Methods[method.Name]; !exists {
				info.PtrMethods[method.Name] = MethodInfo{
					Index: i,
					Name:  method.Name,
					Type:  method.Type,
				}
			}
		}
	}

	return info
}

// collectFields 递归收集字段（包括嵌入字段）
func collectFields(t reflect.Type, fields map[string]FieldInfo, indexPath []int) {
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		if !field.IsExported() {
			continue
		}

		currentIndex := append(indexPath, i)

		if field.Anonymous {
			// 嵌入字段，递归收集
			embeddedType := field.Type
			if embeddedType.Kind() == reflect.Ptr {
				embeddedType = embeddedType.Elem()
			}
			if embeddedType.Kind() == reflect.Struct {
				collectFields(embeddedType, fields, currentIndex)
			}
		}

		// 添加字段（使用小写首字母和原始名）
		fieldInfo := FieldInfo{
			Index:    i,
			Name:     field.Name,
			Type:     field.Type,
			Embedded: field.Anonymous,
		}

		// 小写首字母名
		lowerName := toLowerFirst(field.Name)
		if _, exists := fields[lowerName]; !exists {
			fields[lowerName] = fieldInfo
		}
		// 原始名
		if _, exists := fields[field.Name]; !exists {
			fields[field.Name] = fieldInfo
		}
	}
}

// toLowerFirst 首字母小写
func toLowerFirst(s string) string {
	if len(s) == 0 {
		return s
	}
	if s[0] >= 'A' && s[0] <= 'Z' {
		return string(s[0]+32) + s[1:]
	}
	return s
}

// StringBuilderPool 字符串构建器池
var stringBuilderPool = sync.Pool{
	New: func() interface{} {
		return new(strings.Builder)
	},
}

// getStringBuilder 获取字符串构建器
func getStringBuilder() *strings.Builder {
	sb := stringBuilderPool.Get().(*strings.Builder)
	sb.Reset()
	return sb
}

// putStringBuilder 归还字符串构建器
func putStringBuilder(sb *strings.Builder) {
	stringBuilderPool.Put(sb)
}

// ArgsSlicePool 参数切片池
var argsSlicePool = sync.Pool{
	New: func() interface{} {
		s := make([]interface{}, 0, 16)
		return &s
	},
}

// getArgsSlice 获取参数切片
func getArgsSlice() *[]interface{} {
	s := argsSlicePool.Get().(*[]interface{})
	*s = (*s)[:0]
	return s
}

// putArgsSlice 归还参数切片
func putArgsSlice(s *[]interface{}) {
	argsSlicePool.Put(s)
}

// ScopePool scope 池
var scopePool = sync.Pool{
	New: func() interface{} {
		return make(map[string]interface{}, 16)
	},
}

// getScope 获取 scope
func getScope() map[string]interface{} {
	m := scopePool.Get().(map[string]interface{})
	for k := range m {
		delete(m, k)
	}
	return m
}

// putScope 归还 scope
func putScope(m map[string]interface{}) {
	scopePool.Put(m)
}


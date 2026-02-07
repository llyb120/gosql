package gosql

import (
	"fmt"
	"reflect"
	"strconv"
)

// ==================== 静态模式比较辅助函数 ====================
// 这些函数供生成的静态代码使用，对 interface{} 值做类型安全的比较。
// 不需要 goscript2 解释器，纯 Go 实现。

// GT 大于比较 a > b
func GT(a, b interface{}) bool {
	return ToNumber(a) > ToNumber(b)
}

// GE 大于等于比较 a >= b
func GE(a, b interface{}) bool {
	return ToNumber(a) >= ToNumber(b)
}

// LT 小于比较 a < b
func LT(a, b interface{}) bool {
	return ToNumber(a) < ToNumber(b)
}

// LE 小于等于比较 a <= b
func LE(a, b interface{}) bool {
	return ToNumber(a) <= ToNumber(b)
}

// EQ 等于比较 a == b
func EQ(a, b interface{}) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	// 同类型直接比较
	if reflect.TypeOf(a) == reflect.TypeOf(b) {
		return reflect.DeepEqual(a, b)
	}
	// 数值类型比较
	if isNumericType(a) && isNumericType(b) {
		return ToNumber(a) == ToNumber(b)
	}
	// 字符串比较
	return fmt.Sprint(a) == fmt.Sprint(b)
}

// NE 不等于比较 a != b
func NE(a, b interface{}) bool {
	return !EQ(a, b)
}

// ==================== 类型转换 ====================

// ToBool 将 interface{} 转换为 bool（与 IsTruthy 逻辑一致）
func ToBool(v interface{}) bool {
	if v == nil {
		return false
	}
	rv := reflect.ValueOf(v)
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

// ToNumber 将 interface{} 转换为 float64
func ToNumber(v interface{}) float64 {
	switch val := v.(type) {
	case int:
		return float64(val)
	case int8:
		return float64(val)
	case int16:
		return float64(val)
	case int32:
		return float64(val)
	case int64:
		return float64(val)
	case uint:
		return float64(val)
	case uint8:
		return float64(val)
	case uint16:
		return float64(val)
	case uint32:
		return float64(val)
	case uint64:
		return float64(val)
	case float32:
		return float64(val)
	case float64:
		return val
	case bool:
		if val {
			return 1
		}
		return 0
	case string:
		if f, err := strconv.ParseFloat(val, 64); err == nil {
			return f
		}
		return 0
	default:
		return 0
	}
}

// ==================== 算术运算 ====================

// Add 加法 a + b（保留 int 类型）
func Add(a, b interface{}) interface{} {
	ai, aOk := asInt(a)
	bi, bOk := asInt(b)
	if aOk && bOk {
		return ai + bi
	}
	return ToNumber(a) + ToNumber(b)
}

// Sub 减法 a - b
func Sub(a, b interface{}) interface{} {
	ai, aOk := asInt(a)
	bi, bOk := asInt(b)
	if aOk && bOk {
		return ai - bi
	}
	return ToNumber(a) - ToNumber(b)
}

// Negate 取负 -a
func Negate(v interface{}) interface{} {
	if iv, ok := asInt(v); ok {
		return -iv
	}
	return -ToNumber(v)
}

// Mod 取模 a % b
func Mod(a, b interface{}) interface{} {
	ai, aOk := asInt(a)
	bi, bOk := asInt(b)
	if aOk && bOk && bi != 0 {
		return ai % bi
	}
	return 0
}

// ==================== 集合操作 ====================

// CallLen 获取长度（对应 len()）
func CallLen(v interface{}) int {
	if v == nil {
		return 0
	}
	rv := reflect.ValueOf(v)
	switch rv.Kind() {
	case reflect.Slice, reflect.Array, reflect.Map, reflect.String, reflect.Chan:
		return rv.Len()
	default:
		return 0
	}
}

// Range 遍历 slice/array/map，fn(index, value)
func Range(val interface{}, fn func(idx, val interface{}) error) error {
	if val == nil {
		return nil
	}
	rv := reflect.ValueOf(val)
	switch rv.Kind() {
	case reflect.Slice, reflect.Array:
		for i := 0; i < rv.Len(); i++ {
			if err := fn(i, rv.Index(i).Interface()); err != nil {
				return err
			}
		}
	case reflect.Map:
		for _, key := range rv.MapKeys() {
			if err := fn(key.Interface(), rv.MapIndex(key).Interface()); err != nil {
				return err
			}
		}
	default:
		return fmt.Errorf("cannot range over %s", rv.Kind())
	}
	return nil
}

// ==================== 内部辅助 ====================

func asInt(v interface{}) (int, bool) {
	switch val := v.(type) {
	case int:
		return val, true
	case int8:
		return int(val), true
	case int16:
		return int(val), true
	case int32:
		return int(val), true
	case int64:
		return int(val), true
	case uint:
		return int(val), true
	case uint8:
		return int(val), true
	case uint16:
		return int(val), true
	default:
		return 0, false
	}
}

func isNumericType(v interface{}) bool {
	switch v.(type) {
	case int, int8, int16, int32, int64,
		uint, uint8, uint16, uint32, uint64,
		float32, float64:
		return true
	default:
		return false
	}
}

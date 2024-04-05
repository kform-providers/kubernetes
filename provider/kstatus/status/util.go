package status

import (
	"strings"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// GetStringField return field as string defaulting to value if not found
func GetStringField(obj map[string]interface{}, fieldPath string, defaultValue string) string {
	var rv = defaultValue

	fields := strings.Split(fieldPath, ".")
	if fields[0] == "" {
		fields = fields[1:]
	}

	val, found, err := unstructured.NestedFieldNoCopy(obj, fields...)
	if !found || err != nil {
		return rv
	}

	if v, ok := val.(string); ok {
		return v
	}
	return rv
}

// GetIntField return field as string defaulting to value if not found
func GetIntField(obj map[string]interface{}, fieldPath string, defaultValue int) int {
	fields := strings.Split(fieldPath, ".")
	if fields[0] == "" {
		fields = fields[1:]
	}

	val, found, err := unstructured.NestedFieldNoCopy(obj, fields...)
	if !found || err != nil {
		return defaultValue
	}

	switch v := val.(type) {
	case int:
		return v
	case int32:
		return int(v)
	case int64:
		return int(v)
	}
	return defaultValue
}

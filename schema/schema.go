// Package schema auto-generates JSON Schema (Draft 7) from Go struct types
// using reflection and struct field tags.
//
// Supported struct tags:
//
//   - json:"name"        — field name in the schema; use "-" to skip the field
//   - desc:"..."         — populates the "description" keyword
//   - default:"..."      — populates the "default" keyword; parsed to the field's
//     native type (int, float64, bool, or string)
//   - required:"true"    — explicit marker (redundant: all fields are required
//     unless optional:"true" is also present)
//   - optional:"true"    — exclude the field from the "required" array
//   - enum:"a,b,c"       — populates the "enum" keyword (comma-separated)
//   - min:"1"            — "minimum" for numeric fields
//   - max:"100"          — "maximum" for numeric fields
//   - pattern:"^[a-z]+$" — "pattern" for string fields
package schema

import (
	"reflect"
	"strconv"
	"strings"
	"time"
)

// timeType is a cached reference to reflect.Type of time.Time used throughout
// the package to avoid repeated calls to reflect.TypeOf.
var timeType = reflect.TypeOf(time.Time{})

// Generate returns a JSON Schema (Draft 7) map for the type parameter T.
// T must be a struct type; the schema is produced by inspecting all exported
// fields and their struct tags via reflection.
//
// Example:
//
//	type Input struct {
//	    Name string `json:"name" desc:"User name"`
//	    Age  int    `json:"age"  min:"0" max:"150"`
//	}
//	s := schema.Generate[Input]()
func Generate[T any]() map[string]any {
	var zero T
	t := reflect.TypeOf(zero)
	// Unwrap pointer so Generate[*MyStruct]() works identically to Generate[MyStruct]().
	for t != nil && t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	return GenerateFromType(t)
}

// GenerateFromType returns a JSON Schema (Draft 7) map for the given
// reflect.Type. It is the low-level counterpart to Generate and is useful
// when the concrete type is only known at runtime.
//
// Pointer types are transparently unwrapped. Struct fields tagged with
// json:"-" and unexported fields are silently skipped.
func GenerateFromType(t reflect.Type) map[string]any {
	if t == nil {
		return map[string]any{"type": "object"}
	}
	// Dereference any number of pointer indirections.
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	// Use a fresh seen-set for each top-level call so callers are independent.
	return buildSchemaWithSeen(t, make(map[reflect.Type]bool))
}

// buildSchema is the recursive core that maps a reflect.Type to its JSON Schema
// representation. seen tracks struct types currently on the call stack to break
// circular references; a repeated struct type is emitted as a bare object
// schema rather than expanding it again.
func buildSchema(t reflect.Type) map[string]any {
	return buildSchemaWithSeen(t, make(map[reflect.Type]bool))
}

// buildSchemaWithSeen is the internal recursive implementation. It carries a
// seen map that records every struct type currently being expanded so that
// circular type references (e.g. type Node struct { Children *Node }) produce a
// schema with a bare "type":"object" placeholder instead of infinite recursion.
func buildSchemaWithSeen(t reflect.Type, seen map[reflect.Type]bool) map[string]any {
	// Dereference pointer at every level of recursion so that *string, **int, etc.
	// are all handled uniformly.
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}

	// time.Time is treated as a formatted string regardless of its underlying
	// struct layout.
	if t == timeType {
		return map[string]any{
			"type":   "string",
			"format": "date-time",
		}
	}

	switch t.Kind() {
	case reflect.String:
		return map[string]any{"type": "string"}

	case reflect.Bool:
		return map[string]any{"type": "boolean"}

	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return map[string]any{"type": "integer"}

	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return map[string]any{"type": "integer"}

	case reflect.Float32, reflect.Float64:
		return map[string]any{"type": "number"}

	case reflect.Slice:
		return map[string]any{
			"type":  "array",
			"items": buildSchemaWithSeen(t.Elem(), seen),
		}

	case reflect.Map:
		// Only map[string]V is representable as a JSON Schema object with
		// additionalProperties; other key types fall back to a plain object.
		if t.Key().Kind() == reflect.String {
			return map[string]any{
				"type":                 "object",
				"additionalProperties": buildSchemaWithSeen(t.Elem(), seen),
			}
		}
		return map[string]any{"type": "object"}

	case reflect.Struct:
		return buildStructSchemaWithSeen(t, seen)

	default:
		// Interfaces, channels, funcs, complex numbers, unsafe pointer — emit a
		// bare object schema as the safest fallback.
		return map[string]any{"type": "object"}
	}
}

// buildStructSchema generates the full "object" schema for a struct type,
// iterating its exported fields and applying all supported struct tags.
func buildStructSchema(t reflect.Type) map[string]any {
	return buildStructSchemaWithSeen(t, make(map[reflect.Type]bool))
}

// buildStructSchemaWithSeen is the internal implementation of buildStructSchema
// that threads the seen map through for circular-reference detection.
func buildStructSchemaWithSeen(t reflect.Type, seen map[reflect.Type]bool) map[string]any {
	// Guard against circular references: if we are already expanding this type
	// somewhere up the call stack, emit a plain "object" schema and stop.
	if seen[t] {
		return map[string]any{"type": "object"}
	}
	seen[t] = true
	defer func() { delete(seen, t) }()
	properties := map[string]any{}
	// Use []any so that validate.validateObject's type assertion ([]any) matches
	// without requiring a JSON round-trip.  The JSON Schema spec does not
	// constrain the Go representation of the "required" array, and []any is the
	// natural type that encoding/json produces when unmarshalling into any.
	required := []any{}

	for i := range t.NumField() {
		field := t.Field(i)

		// Skip unexported fields — they are inaccessible to callers anyway.
		if !field.IsExported() {
			continue
		}

		// Resolve the JSON field name and honour json:"-".
		jsonName, skip, omitempty := parseJSONTag(field)
		if skip {
			continue
		}
		// omitempty is noted but, per spec, does not affect the "required" array.
		_ = omitempty

		fieldSchema := buildFieldSchemaWithSeen(field, seen)
		properties[jsonName] = fieldSchema

		// Every field is required unless explicitly tagged optional:"true".
		if field.Tag.Get("optional") != "true" {
			required = append(required, jsonName)
		}
	}

	schema := map[string]any{
		"type":       "object",
		"properties": properties,
	}
	if len(required) > 0 {
		schema["required"] = required
	}
	return schema
}

// buildFieldSchema produces the schema node for a single struct field,
// incorporating all supported struct tags on top of the base type schema.
func buildFieldSchema(field reflect.StructField) map[string]any {
	return buildFieldSchemaWithSeen(field, make(map[reflect.Type]bool))
}

// buildFieldSchemaWithSeen is the internal implementation that threads the seen
// map through for circular-reference detection.
func buildFieldSchemaWithSeen(field reflect.StructField, seen map[reflect.Type]bool) map[string]any {
	ft := field.Type
	// Unwrap pointer so *string behaves like string for type mapping.
	for ft.Kind() == reflect.Pointer {
		ft = ft.Elem()
	}

	// Start with the base schema derived purely from the Go type.
	s := buildSchemaWithSeen(ft, seen)

	// desc — maps to JSON Schema "description".
	if desc := field.Tag.Get("desc"); desc != "" {
		s["description"] = desc
	}

	// enum — comma-separated list of allowed values.
	if rawEnum := field.Tag.Get("enum"); rawEnum != "" {
		parts := strings.Split(rawEnum, ",")
		enumVals := make([]any, 0, len(parts))
		for _, p := range parts {
			trimmed := strings.TrimSpace(p)
			if trimmed != "" {
				enumVals = append(enumVals, trimmed)
			}
		}
		if len(enumVals) > 0 {
			s["enum"] = enumVals
		}
	}

	// min / max — numeric constraints applied to numeric types.
	baseKind := ft.Kind()
	isNumeric := isNumericKind(baseKind)

	if rawMin := field.Tag.Get("min"); rawMin != "" && isNumeric {
		if v, err := strconv.ParseFloat(rawMin, 64); err == nil {
			s["minimum"] = v
		}
	}
	if rawMax := field.Tag.Get("max"); rawMax != "" && isNumeric {
		if v, err := strconv.ParseFloat(rawMax, 64); err == nil {
			s["maximum"] = v
		}
	}

	// pattern — regex constraint applied to string types only.
	if rawPat := field.Tag.Get("pattern"); rawPat != "" && baseKind == reflect.String {
		s["pattern"] = rawPat
	}

	// default — parse to the native type of the field.
	if rawDefault := field.Tag.Get("default"); rawDefault != "" {
		s["default"] = parseDefault(rawDefault, baseKind)
	}

	return s
}

// parseJSONTag extracts the effective field name from a json struct tag.
// It returns:
//
//   - name     — the resolved field name (falls back to field.Name)
//   - skip     — true when the tag value is "-" (field must be omitted)
//   - omitempty — true when the "omitempty" option is present
func parseJSONTag(field reflect.StructField) (name string, skip bool, omitempty bool) {
	tag := field.Tag.Get("json")
	if tag == "" {
		return field.Name, false, false
	}
	if tag == "-" {
		return "", true, false
	}
	parts := strings.Split(tag, ",")
	name = strings.TrimSpace(parts[0])
	if name == "" {
		name = field.Name
	}
	for _, opt := range parts[1:] {
		if strings.TrimSpace(opt) == "omitempty" {
			omitempty = true
		}
	}
	return name, false, omitempty
}

// parseDefault converts the raw string value from a "default" tag into the
// appropriate Go type based on the field's reflect.Kind.  When conversion
// fails the original string is returned unchanged so that the schema remains
// useful even with a mis-typed default.
func parseDefault(raw string, k reflect.Kind) any {
	switch {
	case isIntegerKind(k):
		if v, err := strconv.ParseInt(raw, 10, 64); err == nil {
			return v
		}
		return raw

	case isUnsignedKind(k):
		if v, err := strconv.ParseUint(raw, 10, 64); err == nil {
			return v
		}
		return raw

	case k == reflect.Float32 || k == reflect.Float64:
		if v, err := strconv.ParseFloat(raw, 64); err == nil {
			return v
		}
		return raw

	case k == reflect.Bool:
		if v, err := strconv.ParseBool(raw); err == nil {
			return v
		}
		return raw

	default:
		return raw
	}
}

// isNumericKind reports whether k represents any integer or floating-point type.
func isNumericKind(k reflect.Kind) bool {
	return isIntegerKind(k) || isUnsignedKind(k) || k == reflect.Float32 || k == reflect.Float64
}

// isIntegerKind reports whether k is a signed integer kind.
func isIntegerKind(k reflect.Kind) bool {
	switch k {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return true
	}
	return false
}

// isUnsignedKind reports whether k is an unsigned integer kind.
func isUnsignedKind(k reflect.Kind) bool {
	switch k {
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return true
	}
	return false
}

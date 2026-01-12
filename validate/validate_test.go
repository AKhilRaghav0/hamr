package validate

import (
	"strings"
	"testing"
)

// ---- helpers ----------------------------------------------------------------

// schema builds a minimal JSON Schema object map.
func schema(pairs ...any) map[string]any {
	m := make(map[string]any, len(pairs)/2)
	for i := 0; i+1 < len(pairs); i += 2 {
		m[pairs[i].(string)] = pairs[i+1]
	}
	return m
}

// props wraps a map of property name → sub-schema into the "properties" key.
func props(pairs ...any) map[string]any {
	p := make(map[string]any, len(pairs)/2)
	for i := 0; i+1 < len(pairs); i += 2 {
		p[pairs[i].(string)] = pairs[i+1]
	}
	return schema("properties", p)
}

// propsWithRequired builds a schema with both "properties" and "required".
func propsWithRequired(required []string, pairs ...any) map[string]any {
	s := props(pairs...)
	reqAny := make([]any, len(required))
	for i, r := range required {
		reqAny[i] = r
	}
	s["required"] = reqAny
	return s
}

// data builds a map[string]any from alternating key/value pairs.
func data(pairs ...any) map[string]any {
	m := make(map[string]any, len(pairs)/2)
	for i := 0; i+1 < len(pairs); i += 2 {
		m[pairs[i].(string)] = pairs[i+1]
	}
	return m
}

// assertNoErrors fails the test if errs is non-nil.
func assertNoErrors(t *testing.T, errs ValidationErrors) {
	t.Helper()
	if errs != nil {
		t.Errorf("expected no errors, got: %s", errs.Error())
	}
}

// assertErrorCount fails if the number of errors differs from want.
func assertErrorCount(t *testing.T, errs ValidationErrors, want int) {
	t.Helper()
	if len(errs) != want {
		t.Errorf("expected %d error(s), got %d: %s", want, len(errs), errs.Error())
	}
}

// assertContains fails if none of the errors contain the substring.
func assertContains(t *testing.T, errs ValidationErrors, substr string) {
	t.Helper()
	for _, e := range errs {
		if strings.Contains(e.Message, substr) {
			return
		}
	}
	t.Errorf("expected an error containing %q, got: %s", substr, errs.Error())
}

// assertField fails if none of the errors reference the given field.
func assertField(t *testing.T, errs ValidationErrors, field string) {
	t.Helper()
	for _, e := range errs {
		if e.Field == field {
			return
		}
	}
	t.Errorf("expected an error for field %q, got: %s", field, errs.Error())
}

// ---- 1. Required field validation -------------------------------------------

func TestRequiredField_Present(t *testing.T) {
	s := propsWithRequired([]string{"query"},
		"query", schema("type", "string"),
	)
	errs := Validate(s, data("query", "hello"))
	assertNoErrors(t, errs)
}

func TestRequiredField_Missing(t *testing.T) {
	s := propsWithRequired([]string{"query"},
		"query", schema("type", "string"),
	)
	errs := Validate(s, data())
	assertErrorCount(t, errs, 1)
	assertContains(t, errs, "field 'query' is required")
	assertField(t, errs, "query")
}

func TestRequiredField_MultipleFields_AllMissing(t *testing.T) {
	s := propsWithRequired([]string{"name", "email"},
		"name", schema("type", "string"),
		"email", schema("type", "string"),
	)
	errs := Validate(s, data())
	assertErrorCount(t, errs, 2)
	assertField(t, errs, "name")
	assertField(t, errs, "email")
}

func TestRequiredField_PartiallyMissing(t *testing.T) {
	s := propsWithRequired([]string{"name", "email"},
		"name", schema("type", "string"),
		"email", schema("type", "string"),
	)
	errs := Validate(s, data("name", "Alice"))
	assertErrorCount(t, errs, 1)
	assertField(t, errs, "email")
}

// ---- 2. Type checking -------------------------------------------------------

func TestTypeCheck_String_Valid(t *testing.T) {
	s := props("name", schema("type", "string"))
	assertNoErrors(t, Validate(s, data("name", "Alice")))
}

func TestTypeCheck_String_Invalid(t *testing.T) {
	s := props("name", schema("type", "string"))
	errs := Validate(s, data("name", 42.0))
	assertErrorCount(t, errs, 1)
	assertContains(t, errs, "must be a string")
	assertContains(t, errs, "got number")
}

func TestTypeCheck_Integer_ValidFloat64(t *testing.T) {
	// JSON numbers arrive as float64.
	s := props("count", schema("type", "integer"))
	assertNoErrors(t, Validate(s, data("count", float64(5))))
}

func TestTypeCheck_Integer_ValidInt(t *testing.T) {
	s := props("count", schema("type", "integer"))
	assertNoErrors(t, Validate(s, data("count", int(5))))
}

func TestTypeCheck_Integer_InvalidFractional(t *testing.T) {
	s := props("count", schema("type", "integer"))
	errs := Validate(s, data("count", 3.14))
	assertErrorCount(t, errs, 1)
	assertContains(t, errs, "must be an integer")
}

func TestTypeCheck_Integer_InvalidString(t *testing.T) {
	s := props("max_results", schema("type", "integer"))
	errs := Validate(s, data("max_results", "ten"))
	assertErrorCount(t, errs, 1)
	assertContains(t, errs, "field 'max_results' must be an integer, got string")
}

func TestTypeCheck_Number_ValidFloat(t *testing.T) {
	s := props("price", schema("type", "number"))
	assertNoErrors(t, Validate(s, data("price", 9.99)))
}

func TestTypeCheck_Number_InvalidString(t *testing.T) {
	s := props("price", schema("type", "number"))
	errs := Validate(s, data("price", "cheap"))
	assertErrorCount(t, errs, 1)
	assertContains(t, errs, "must be a number")
}

func TestTypeCheck_Boolean_Valid(t *testing.T) {
	s := props("active", schema("type", "boolean"))
	assertNoErrors(t, Validate(s, data("active", true)))
}

func TestTypeCheck_Boolean_InvalidString(t *testing.T) {
	s := props("active", schema("type", "boolean"))
	errs := Validate(s, data("active", "true"))
	assertErrorCount(t, errs, 1)
	assertContains(t, errs, "must be a boolean")
	assertContains(t, errs, "got string")
}

func TestTypeCheck_Array_Valid(t *testing.T) {
	s := props("tags", schema("type", "array"))
	assertNoErrors(t, Validate(s, data("tags", []any{"a", "b"})))
}

func TestTypeCheck_Array_InvalidString(t *testing.T) {
	s := props("tags", schema("type", "array"))
	errs := Validate(s, data("tags", "not-an-array"))
	assertErrorCount(t, errs, 1)
	assertContains(t, errs, "must be an array")
}

func TestTypeCheck_Object_Valid(t *testing.T) {
	s := props("meta", schema("type", "object"))
	assertNoErrors(t, Validate(s, data("meta", map[string]any{"k": "v"})))
}

func TestTypeCheck_Object_InvalidString(t *testing.T) {
	s := props("meta", schema("type", "object"))
	errs := Validate(s, data("meta", "{}"))
	assertErrorCount(t, errs, 1)
	assertContains(t, errs, "must be an object")
}

// ---- 3. Enum validation -----------------------------------------------------

func TestEnum_ValidValue(t *testing.T) {
	s := props("format", schema("type", "string", "enum", []any{"json", "xml", "csv"}))
	assertNoErrors(t, Validate(s, data("format", "json")))
}

func TestEnum_InvalidValue(t *testing.T) {
	s := props("format", schema("type", "string", "enum", []any{"json", "xml", "csv"}))
	errs := Validate(s, data("format", "yaml"))
	assertErrorCount(t, errs, 1)
	assertContains(t, errs, "field 'format' must be one of [json, xml, csv], got 'yaml'")
}

func TestEnum_NumericValues(t *testing.T) {
	s := props("code", schema("type", "integer", "enum", []any{float64(1), float64(2), float64(3)}))
	assertNoErrors(t, Validate(s, data("code", float64(2))))
}

func TestEnum_NumericValues_Invalid(t *testing.T) {
	s := props("code", schema("type", "integer", "enum", []any{float64(1), float64(2), float64(3)}))
	errs := Validate(s, data("code", float64(99)))
	assertErrorCount(t, errs, 1)
	assertContains(t, errs, "must be one of")
}

// ---- 4. Min/Max validation --------------------------------------------------

func TestMinimum_Valid(t *testing.T) {
	s := props("limit", schema("type", "integer", "minimum", float64(1)))
	assertNoErrors(t, Validate(s, data("limit", float64(5))))
}

func TestMinimum_AtBoundary(t *testing.T) {
	s := props("limit", schema("type", "integer", "minimum", float64(1)))
	assertNoErrors(t, Validate(s, data("limit", float64(1))))
}

func TestMinimum_Invalid(t *testing.T) {
	s := props("limit", schema("type", "integer", "minimum", float64(1)))
	errs := Validate(s, data("limit", float64(0)))
	assertErrorCount(t, errs, 1)
	assertContains(t, errs, "field 'limit' must be >= 1, got 0")
}

func TestMaximum_Valid(t *testing.T) {
	s := props("page_size", schema("type", "integer", "maximum", float64(100)))
	assertNoErrors(t, Validate(s, data("page_size", float64(50))))
}

func TestMaximum_AtBoundary(t *testing.T) {
	s := props("page_size", schema("type", "integer", "maximum", float64(100)))
	assertNoErrors(t, Validate(s, data("page_size", float64(100))))
}

func TestMaximum_Invalid(t *testing.T) {
	s := props("page_size", schema("type", "integer", "maximum", float64(100)))
	errs := Validate(s, data("page_size", float64(200)))
	assertErrorCount(t, errs, 1)
	assertContains(t, errs, "must be <= 100")
	assertContains(t, errs, "got 200")
}

func TestMinMax_Both_Valid(t *testing.T) {
	s := props("score", schema("type", "number", "minimum", float64(0), "maximum", float64(1)))
	assertNoErrors(t, Validate(s, data("score", 0.5)))
}

func TestMinMax_BothViolated(t *testing.T) {
	// Each constraint fires independently.
	s := props("score", schema("type", "number", "minimum", float64(0), "maximum", float64(1)))
	// Below minimum.
	errs := Validate(s, data("score", -1.0))
	assertErrorCount(t, errs, 1)
	assertContains(t, errs, ">= 0")
}

// ---- 5. Pattern validation --------------------------------------------------

func TestPattern_Valid(t *testing.T) {
	s := props("name", schema("type", "string", "pattern", `^[a-z]+$`))
	assertNoErrors(t, Validate(s, data("name", "alice")))
}

func TestPattern_Invalid(t *testing.T) {
	s := props("name", schema("type", "string", "pattern", `^[a-z]+$`))
	errs := Validate(s, data("name", "Alice123"))
	assertErrorCount(t, errs, 1)
	assertContains(t, errs, "field 'name' must match pattern ^[a-z]+$")
}

func TestPattern_EmailLike(t *testing.T) {
	s := props("email", schema("type", "string", "pattern", `^[^@]+@[^@]+\.[^@]+$`))
	assertNoErrors(t, Validate(s, data("email", "user@example.com")))
}

func TestPattern_EmailLike_Invalid(t *testing.T) {
	s := props("email", schema("type", "string", "pattern", `^[^@]+@[^@]+\.[^@]+$`))
	errs := Validate(s, data("email", "not-an-email"))
	assertErrorCount(t, errs, 1)
}

func TestPattern_InvalidRegex_NoError(t *testing.T) {
	// An invalid regex should not panic; it is treated as no constraint.
	s := props("x", schema("type", "string", "pattern", `[invalid`))
	assertNoErrors(t, Validate(s, data("x", "anything")))
}

// ---- 6. Nested object validation --------------------------------------------

func TestNestedObject_Valid(t *testing.T) {
	inner := propsWithRequired([]string{"street"},
		"street", schema("type", "string"),
		"zip", schema("type", "string"),
	)
	inner["type"] = "object"
	s := props("address", inner)
	d := data("address", map[string]any{"street": "Main St", "zip": "12345"})
	assertNoErrors(t, Validate(s, d))
}

func TestNestedObject_MissingRequired(t *testing.T) {
	inner := propsWithRequired([]string{"street"},
		"street", schema("type", "string"),
	)
	inner["type"] = "object"
	s := props("address", inner)
	d := data("address", map[string]any{})
	errs := Validate(s, d)
	assertErrorCount(t, errs, 1)
	assertField(t, errs, "address.street")
	assertContains(t, errs, "field 'address.street' is required")
}

func TestNestedObject_TypeMismatch(t *testing.T) {
	inner := schema("type", "object", "properties", map[string]any{
		"zip": schema("type", "integer"),
	})
	s := props("address", inner)
	d := data("address", map[string]any{"zip": "not-a-number"})
	errs := Validate(s, d)
	assertErrorCount(t, errs, 1)
	assertField(t, errs, "address.zip")
}

func TestNestedObject_DeeplyNested(t *testing.T) {
	deepest := propsWithRequired([]string{"value"},
		"value", schema("type", "string"),
	)
	deepest["type"] = "object"
	middle := schema("type", "object", "properties", map[string]any{
		"deep": deepest,
	})
	s := props("a", middle)
	d := data("a", map[string]any{
		"deep": map[string]any{}, // missing "value"
	})
	errs := Validate(s, d)
	assertErrorCount(t, errs, 1)
	assertField(t, errs, "a.deep.value")
}

// ---- 7. Array item validation -----------------------------------------------

func TestArrayItems_Valid(t *testing.T) {
	s := props("tags", schema(
		"type", "array",
		"items", schema("type", "string"),
	))
	assertNoErrors(t, Validate(s, data("tags", []any{"foo", "bar"})))
}

func TestArrayItems_InvalidItem(t *testing.T) {
	s := props("tags", schema(
		"type", "array",
		"items", schema("type", "string"),
	))
	errs := Validate(s, data("tags", []any{"foo", 42.0, "bar"}))
	assertErrorCount(t, errs, 1)
	assertField(t, errs, "tags[1]")
	assertContains(t, errs, "must be a string")
}

func TestArrayItems_MultipleInvalidItems(t *testing.T) {
	s := props("ids", schema(
		"type", "array",
		"items", schema("type", "integer"),
	))
	errs := Validate(s, data("ids", []any{"a", "b", float64(3)}))
	assertErrorCount(t, errs, 2)
	assertField(t, errs, "ids[0]")
	assertField(t, errs, "ids[1]")
}

func TestArrayItems_EmptyArray(t *testing.T) {
	s := props("tags", schema(
		"type", "array",
		"items", schema("type", "string"),
	))
	assertNoErrors(t, Validate(s, data("tags", []any{})))
}

func TestArrayItems_MinMaxOnItems(t *testing.T) {
	s := props("scores", schema(
		"type", "array",
		"items", schema("type", "number", "minimum", float64(0), "maximum", float64(100)),
	))
	errs := Validate(s, data("scores", []any{50.0, -1.0, 101.0}))
	assertErrorCount(t, errs, 2)
	assertField(t, errs, "scores[1]")
	assertField(t, errs, "scores[2]")
}

func TestArrayItems_ObjectItems(t *testing.T) {
	itemSchema := propsWithRequired([]string{"id"},
		"id", schema("type", "integer"),
	)
	itemSchema["type"] = "object"
	s := props("records", schema(
		"type", "array",
		"items", itemSchema,
	))
	d := data("records", []any{
		map[string]any{"id": float64(1)},
		map[string]any{},          // missing "id"
		map[string]any{"id": "x"}, // wrong type
	})
	errs := Validate(s, d)
	// index 1: required, index 2: type error
	assertErrorCount(t, errs, 2)
}

// ---- 8. ApplyDefaults -------------------------------------------------------

func TestApplyDefaults_MissingField(t *testing.T) {
	s := props(
		"format", schema("type", "string", "default", "json"),
		"limit", schema("type", "integer", "default", float64(10)),
	)
	d := ApplyDefaults(s, data())
	if d["format"] != "json" {
		t.Errorf("expected format=json, got %v", d["format"])
	}
	if d["limit"] != float64(10) {
		t.Errorf("expected limit=10, got %v", d["limit"])
	}
}

func TestApplyDefaults_ExistingFieldNotOverwritten(t *testing.T) {
	s := props("limit", schema("type", "integer", "default", float64(10)))
	d := ApplyDefaults(s, data("limit", float64(50)))
	if d["limit"] != float64(50) {
		t.Errorf("expected limit=50 (user value), got %v", d["limit"])
	}
}

func TestApplyDefaults_NoDefault_FieldStaysAbsent(t *testing.T) {
	s := props("query", schema("type", "string"))
	d := ApplyDefaults(s, data())
	if _, ok := d["query"]; ok {
		t.Errorf("expected 'query' to be absent when there is no default")
	}
}

func TestApplyDefaults_NestedObject(t *testing.T) {
	inner := schema(
		"type", "object",
		"properties", map[string]any{
			"page_size": schema("type", "integer", "default", float64(20)),
		},
	)
	s := props("pagination", inner)
	nested := map[string]any{} // exists but missing page_size
	d := ApplyDefaults(s, data("pagination", nested))

	pagination, ok := d["pagination"].(map[string]any)
	if !ok {
		t.Fatal("expected 'pagination' to be a map")
	}
	if pagination["page_size"] != float64(20) {
		t.Errorf("expected page_size=20, got %v", pagination["page_size"])
	}
}

func TestApplyDefaults_NilData(t *testing.T) {
	s := props("format", schema("type", "string", "default", "json"))
	d := ApplyDefaults(s, nil)
	if d == nil {
		t.Fatal("expected non-nil map")
	}
	if d["format"] != "json" {
		t.Errorf("expected format=json, got %v", d["format"])
	}
}

func TestApplyDefaults_ReturnsData(t *testing.T) {
	s := props("x", schema("type", "string", "default", "hello"))
	in := data()
	out := ApplyDefaults(s, in)
	// Must be the same map modified in-place.
	if out["x"] != "hello" {
		t.Errorf("expected x=hello, got %v", out["x"])
	}
	if in["x"] != "hello" {
		t.Errorf("expected in-place modification")
	}
}

// ---- 9. Multiple errors at once ---------------------------------------------

func TestMultipleErrors(t *testing.T) {
	s := propsWithRequired([]string{"query", "limit"},
		"query", schema("type", "string"),
		"limit", schema("type", "integer", "minimum", float64(1)),
		"format", schema("type", "string", "enum", []any{"json", "xml"}),
	)
	d := data(
		"limit", float64(0),   // violates minimum
		"format", "yaml",      // violates enum
		// "query" missing — required error
	)
	errs := Validate(s, d)
	if len(errs) < 3 {
		t.Errorf("expected at least 3 errors, got %d: %s", len(errs), errs.Error())
	}
	assertField(t, errs, "query")
	assertField(t, errs, "limit")
	assertField(t, errs, "format")
}

// ---- 10. Valid data passes --------------------------------------------------

func TestValidData_AllTypesPresent(t *testing.T) {
	s := propsWithRequired([]string{"name", "age", "score", "active", "tags"},
		"name", schema("type", "string", "pattern", `^\w+$`),
		"age", schema("type", "integer", "minimum", float64(0), "maximum", float64(150)),
		"score", schema("type", "number", "minimum", float64(0.0), "maximum", float64(1.0)),
		"active", schema("type", "boolean"),
		"tags", schema("type", "array", "items", schema("type", "string")),
		"format", schema("type", "string", "enum", []any{"json", "xml"}),
	)
	d := data(
		"name", "alice",
		"age", float64(30),
		"score", 0.95,
		"active", true,
		"tags", []any{"go", "mcp"},
		"format", "json",
	)
	assertNoErrors(t, Validate(s, d))
}

// ---- 11. Empty schema accepts anything --------------------------------------

func TestEmptySchema_AcceptsAnything(t *testing.T) {
	s := map[string]any{}
	d := data("arbitrary_field", "value", "another", 42.0)
	assertNoErrors(t, Validate(s, d))
}

func TestEmptySchema_EmptyData(t *testing.T) {
	assertNoErrors(t, Validate(map[string]any{}, data()))
}

func TestNoProperties_AcceptsAnything(t *testing.T) {
	// Schema has a type but no properties — should not error on extra fields.
	s := schema("type", "object")
	assertNoErrors(t, Validate(s, data("x", "y")))
}

// ---- 12. Human-readable error messages -------------------------------------

func TestErrorMessage_RequiredField(t *testing.T) {
	s := propsWithRequired([]string{"query"}, "query", schema("type", "string"))
	errs := Validate(s, data())
	assertContains(t, errs, "field 'query' is required")
}

func TestErrorMessage_TypeMismatch_IntegerGotString(t *testing.T) {
	s := props("max_results", schema("type", "integer"))
	errs := Validate(s, data("max_results", "ten"))
	assertContains(t, errs, "field 'max_results' must be an integer, got string")
}

func TestErrorMessage_Enum(t *testing.T) {
	s := props("format", schema("type", "string", "enum", []any{"json", "xml", "csv"}))
	errs := Validate(s, data("format", "yaml"))
	assertContains(t, errs, "field 'format' must be one of [json, xml, csv], got 'yaml'")
}

func TestErrorMessage_Minimum(t *testing.T) {
	s := props("limit", schema("type", "integer", "minimum", float64(1)))
	errs := Validate(s, data("limit", float64(0)))
	assertContains(t, errs, "field 'limit' must be >= 1, got 0")
}

func TestErrorMessage_Maximum(t *testing.T) {
	s := props("count", schema("type", "integer", "maximum", float64(100)))
	errs := Validate(s, data("count", float64(200)))
	assertContains(t, errs, "field 'count' must be <= 100, got 200")
}

func TestErrorMessage_Pattern(t *testing.T) {
	s := props("name", schema("type", "string", "pattern", `^[a-z]+$`))
	errs := Validate(s, data("name", "UPPER"))
	assertContains(t, errs, "field 'name' must match pattern ^[a-z]+$")
}

func TestErrorMessage_NestedField(t *testing.T) {
	inner := propsWithRequired([]string{"street"},
		"street", schema("type", "string"),
	)
	inner["type"] = "object"
	s := props("address", inner)
	errs := Validate(s, data("address", map[string]any{}))
	assertContains(t, errs, "field 'address.street' is required")
}

func TestErrorMessage_ArrayItemField(t *testing.T) {
	s := props("ids", schema(
		"type", "array",
		"items", schema("type", "integer"),
	))
	errs := Validate(s, data("ids", []any{float64(1), "oops"}))
	assertContains(t, errs, "ids[1]")
}

// ---- ValidationErrors.Error() -----------------------------------------------

func TestValidationErrors_Error_Single(t *testing.T) {
	errs := ValidationErrors{{Field: "x", Message: "field 'x' is required"}}
	if errs.Error() != "field 'x' is required" {
		t.Errorf("unexpected: %s", errs.Error())
	}
}

func TestValidationErrors_Error_Multiple(t *testing.T) {
	errs := ValidationErrors{
		{Field: "a", Message: "msg a"},
		{Field: "b", Message: "msg b"},
	}
	got := errs.Error()
	if !strings.Contains(got, "msg a") || !strings.Contains(got, "msg b") {
		t.Errorf("unexpected: %s", got)
	}
}

func TestValidationErrors_Error_Empty(t *testing.T) {
	var errs ValidationErrors
	if errs.Error() != "" {
		t.Errorf("expected empty string for empty errors, got %q", errs.Error())
	}
}

// ---- ValidationError.Error() ------------------------------------------------

func TestValidationError_Error(t *testing.T) {
	e := &ValidationError{Field: "foo", Message: "field 'foo' is required"}
	if e.Error() != "field 'foo' is required" {
		t.Errorf("unexpected: %s", e.Error())
	}
}

// ---- Edge cases -------------------------------------------------------------

func TestValidate_ReturnsNil_OnSuccess(t *testing.T) {
	s := props("x", schema("type", "string"))
	errs := Validate(s, data("x", "hello"))
	if errs != nil {
		t.Errorf("expected nil, got %v", errs)
	}
}

func TestValidate_FloatMinimumMessage(t *testing.T) {
	s := props("ratio", schema("type", "number", "minimum", 0.5))
	errs := Validate(s, data("ratio", 0.1))
	assertErrorCount(t, errs, 1)
	assertContains(t, errs, ">= 0.5")
}

func TestTypeCheck_Integer_WholeFractionFloat(t *testing.T) {
	// float64(5.0) should pass integer check.
	s := props("n", schema("type", "integer"))
	assertNoErrors(t, Validate(s, data("n", float64(5.0))))
}

func TestEnum_BoolValues(t *testing.T) {
	s := props("flag", schema("type", "boolean", "enum", []any{true}))
	assertNoErrors(t, Validate(s, data("flag", true)))
	errs := Validate(s, data("flag", false))
	assertErrorCount(t, errs, 1)
}

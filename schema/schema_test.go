package schema_test

import (
	"reflect"
	"testing"
	"time"

	"github.com/AKhilRaghav0/hamr/schema"
)

// ---- helpers ----------------------------------------------------------------

// assertType fails the test if the "type" key in s does not match want.
func assertType(t *testing.T, s map[string]any, want string) {
	t.Helper()
	got, ok := s["type"]
	if !ok {
		t.Fatalf("schema missing \"type\" key; schema=%v", s)
	}
	if got != want {
		t.Fatalf("type: got %q, want %q", got, want)
	}
}

// prop extracts s["properties"][name] and fails if either key is absent.
func prop(t *testing.T, s map[string]any, name string) map[string]any {
	t.Helper()
	rawProps, ok := s["properties"]
	if !ok {
		t.Fatalf("schema has no \"properties\" key; schema=%v", s)
	}
	props, ok := rawProps.(map[string]any)
	if !ok {
		t.Fatalf("\"properties\" is not map[string]any; got %T", rawProps)
	}
	rawField, ok := props[name]
	if !ok {
		t.Fatalf("property %q not found; available=%v", name, keys(props))
	}
	field, ok := rawField.(map[string]any)
	if !ok {
		t.Fatalf("property %q is not map[string]any; got %T", name, rawField)
	}
	return field
}

// hasRequired returns true if name appears in s["required"].
func hasRequired(s map[string]any, name string) bool {
	raw, ok := s["required"]
	if !ok {
		return false
	}
	switch v := raw.(type) {
	case []string:
		for _, r := range v {
			if r == name {
				return true
			}
		}
	case []any:
		for _, r := range v {
			if r == name {
				return true
			}
		}
	}
	return false
}

// keys returns a slice of keys from a map[string]any for error messages.
func keys(m map[string]any) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

// ---- 1. Basic scalar types --------------------------------------------------

type BasicTypes struct {
	S   string  `json:"s"`
	I   int     `json:"i"`
	F   float64 `json:"f"`
	B   bool    `json:"b"`
	I8  int8    `json:"i8"`
	U   uint    `json:"u"`
	F32 float32 `json:"f32"`
}

func TestBasicTypes(t *testing.T) {
	s := schema.Generate[BasicTypes]()
	assertType(t, s, "object")

	cases := []struct {
		field    string
		wantType string
	}{
		{"s", "string"},
		{"i", "integer"},
		{"f", "number"},
		{"b", "boolean"},
		{"i8", "integer"},
		{"u", "integer"},
		{"f32", "number"},
	}
	for _, c := range cases {
		t.Run(c.field, func(t *testing.T) {
			assertType(t, prop(t, s, c.field), c.wantType)
		})
	}
}

// ---- 2a. desc tag -----------------------------------------------------------

type DescTag struct {
	Name string `json:"name" desc:"The user's full name"`
}

func TestDescTag(t *testing.T) {
	s := schema.Generate[DescTag]()
	p := prop(t, s, "name")
	if got := p["description"]; got != "The user's full name" {
		t.Fatalf("description: got %v, want %q", got, "The user's full name")
	}
}

// ---- 2b. default tag --------------------------------------------------------

type DefaultTags struct {
	Label  string  `json:"label"   default:"hello"`
	Count  int     `json:"count"   default:"42"`
	Score  float64 `json:"score"   default:"3.14"`
	Active bool    `json:"active"  default:"true"`
	Size   uint    `json:"size"    default:"10"`
}

func TestDefaultTag(t *testing.T) {
	s := schema.Generate[DefaultTags]()

	cases := []struct {
		field string
		want  any
	}{
		{"label", "hello"},
		{"count", int64(42)},
		{"score", 3.14},
		{"active", true},
		{"size", uint64(10)},
	}
	for _, c := range cases {
		t.Run(c.field, func(t *testing.T) {
			p := prop(t, s, c.field)
			got, ok := p["default"]
			if !ok {
				t.Fatalf("field %q: missing \"default\" key", c.field)
			}
			if !reflect.DeepEqual(got, c.want) {
				t.Fatalf("field %q: default got %v (%T), want %v (%T)",
					c.field, got, got, c.want, c.want)
			}
		})
	}
}

// ---- 2c. enum tag -----------------------------------------------------------

type EnumTag struct {
	Color string `json:"color" enum:"red,green,blue"`
}

func TestEnumTag(t *testing.T) {
	s := schema.Generate[EnumTag]()
	p := prop(t, s, "color")
	rawEnum, ok := p["enum"]
	if !ok {
		t.Fatal("property \"color\": missing \"enum\" key")
	}
	enumSlice, ok := rawEnum.([]any)
	if !ok {
		t.Fatalf("\"enum\" is not []any; got %T", rawEnum)
	}
	want := []any{"red", "green", "blue"}
	if !reflect.DeepEqual(enumSlice, want) {
		t.Fatalf("enum: got %v, want %v", enumSlice, want)
	}
}

// ---- 2d. min / max tags -----------------------------------------------------

type MinMaxTag struct {
	Age   int     `json:"age"   min:"0"   max:"150"`
	Score float64 `json:"score" min:"0.0" max:"100.0"`
}

func TestMinMaxTag(t *testing.T) {
	s := schema.Generate[MinMaxTag]()

	age := prop(t, s, "age")
	if v, ok := age["minimum"]; !ok || v != float64(0) {
		t.Fatalf("age.minimum: got %v, want 0", v)
	}
	if v, ok := age["maximum"]; !ok || v != float64(150) {
		t.Fatalf("age.maximum: got %v, want 150", v)
	}

	score := prop(t, s, "score")
	if v, ok := score["minimum"]; !ok || v != float64(0) {
		t.Fatalf("score.minimum: got %v, want 0.0", v)
	}
	if v, ok := score["maximum"]; !ok || v != float64(100) {
		t.Fatalf("score.maximum: got %v, want 100.0", v)
	}
}

// ---- 2e. pattern tag --------------------------------------------------------

type PatternTag struct {
	Slug string `json:"slug" pattern:"^[a-z0-9-]+$"`
}

func TestPatternTag(t *testing.T) {
	s := schema.Generate[PatternTag]()
	p := prop(t, s, "slug")
	if got := p["pattern"]; got != "^[a-z0-9-]+$" {
		t.Fatalf("pattern: got %v, want %q", got, "^[a-z0-9-]+$")
	}
}

// min/max should NOT be emitted for string fields.
func TestMinMaxNotAppliedToStrings(t *testing.T) {
	type S struct {
		Token string `json:"token" min:"1" max:"10"`
	}
	s := schema.Generate[S]()
	p := prop(t, s, "token")
	if _, ok := p["minimum"]; ok {
		t.Fatal("\"minimum\" should not appear on a string field")
	}
	if _, ok := p["maximum"]; ok {
		t.Fatal("\"maximum\" should not appear on a string field")
	}
}

// pattern should NOT be emitted for non-string fields.
func TestPatternNotAppliedToInts(t *testing.T) {
	type S struct {
		N int `json:"n" pattern:"^[0-9]+$"`
	}
	s := schema.Generate[S]()
	p := prop(t, s, "n")
	if _, ok := p["pattern"]; ok {
		t.Fatal("\"pattern\" should not appear on an integer field")
	}
}

// ---- 2f. required / optional tags ------------------------------------------

type RequiredOptional struct {
	// All fields are required by default.
	Alpha string `json:"alpha"`
	// Explicit required:"true" is redundant but valid.
	Beta string `json:"beta" required:"true"`
	// optional:"true" removes from required array.
	Gamma string `json:"gamma" optional:"true"`
}

func TestRequiredOptionalTags(t *testing.T) {
	s := schema.Generate[RequiredOptional]()

	if !hasRequired(s, "alpha") {
		t.Error("\"alpha\" should be required (default behaviour)")
	}
	if !hasRequired(s, "beta") {
		t.Error("\"beta\" should be required (explicit required:\"true\")")
	}
	if hasRequired(s, "gamma") {
		t.Error("\"gamma\" must NOT be required (optional:\"true\")")
	}
}

// ---- 3. Nested structs ------------------------------------------------------

type Address struct {
	Street string `json:"street"`
	City   string `json:"city" desc:"City name"`
}

type Person struct {
	Name    string  `json:"name"`
	Address Address `json:"address"`
}

func TestNestedStruct(t *testing.T) {
	s := schema.Generate[Person]()
	assertType(t, s, "object")

	addrSchema := prop(t, s, "address")
	assertType(t, addrSchema, "object")

	street := prop(t, addrSchema, "street")
	assertType(t, street, "string")

	city := prop(t, addrSchema, "city")
	assertType(t, city, "string")
	if city["description"] != "City name" {
		t.Fatalf("nested desc: got %v", city["description"])
	}
}

// Required propagates correctly into nested struct.
func TestNestedStructRequired(t *testing.T) {
	s := schema.Generate[Person]()
	if !hasRequired(s, "name") {
		t.Error("\"name\" should be required on Person")
	}
	if !hasRequired(s, "address") {
		t.Error("\"address\" should be required on Person")
	}

	addrSchema := prop(t, s, "address")
	if !hasRequired(addrSchema, "street") {
		t.Error("\"street\" should be required on Address")
	}
	if !hasRequired(addrSchema, "city") {
		t.Error("\"city\" should be required on Address")
	}
}

// ---- 4. Slices and maps -----------------------------------------------------

type Containers struct {
	Tags    []string          `json:"tags"`
	Scores  []int             `json:"scores"`
	Meta    map[string]string `json:"meta"`
	Counter map[string]int    `json:"counter"`
}

func TestSlices(t *testing.T) {
	s := schema.Generate[Containers]()

	tags := prop(t, s, "tags")
	assertType(t, tags, "array")
	items, ok := tags["items"].(map[string]any)
	if !ok {
		t.Fatalf("tags.items is not map[string]any; got %T", tags["items"])
	}
	assertType(t, items, "string")

	scores := prop(t, s, "scores")
	assertType(t, scores, "array")
	scoreItems, ok := scores["items"].(map[string]any)
	if !ok {
		t.Fatalf("scores.items is not map[string]any; got %T", scores["items"])
	}
	assertType(t, scoreItems, "integer")
}

func TestMaps(t *testing.T) {
	s := schema.Generate[Containers]()

	meta := prop(t, s, "meta")
	assertType(t, meta, "object")
	addlProps, ok := meta["additionalProperties"].(map[string]any)
	if !ok {
		t.Fatalf("meta.additionalProperties is not map[string]any; got %T", meta["additionalProperties"])
	}
	assertType(t, addlProps, "string")

	counter := prop(t, s, "counter")
	assertType(t, counter, "object")
	counterProps, ok := counter["additionalProperties"].(map[string]any)
	if !ok {
		t.Fatalf("counter.additionalProperties is not map[string]any; got %T", counter["additionalProperties"])
	}
	assertType(t, counterProps, "integer")
}

// ---- 5. Pointer types -------------------------------------------------------

type WithPointers struct {
	Name  *string  `json:"name"`
	Age   *int     `json:"age"`
	Score *float64 `json:"score"`
	Flag  *bool    `json:"flag"`
}

func TestPointerTypes(t *testing.T) {
	s := schema.Generate[WithPointers]()

	cases := []struct {
		field    string
		wantType string
	}{
		{"name", "string"},
		{"age", "integer"},
		{"score", "number"},
		{"flag", "boolean"},
	}
	for _, c := range cases {
		t.Run(c.field, func(t *testing.T) {
			assertType(t, prop(t, s, c.field), c.wantType)
		})
	}
}

// Pointer to struct should produce a nested object schema.
func TestPointerToStruct(t *testing.T) {
	type Inner struct {
		X int `json:"x"`
	}
	type Outer struct {
		Inner *Inner `json:"inner"`
	}
	s := schema.Generate[Outer]()
	innerSchema := prop(t, s, "inner")
	assertType(t, innerSchema, "object")
	assertType(t, prop(t, innerSchema, "x"), "integer")
}

// Generate itself accepts a pointer type parameter.
func TestGeneratePointerTypeParam(t *testing.T) {
	type Simple struct {
		ID int `json:"id"`
	}
	s := schema.Generate[*Simple]()
	assertType(t, s, "object")
	assertType(t, prop(t, s, "id"), "integer")
}

// ---- 6. time.Time -----------------------------------------------------------

type WithTime struct {
	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt *time.Time `json:"updated_at"`
}

func TestTimeType(t *testing.T) {
	s := schema.Generate[WithTime]()

	for _, field := range []string{"created_at", "updated_at"} {
		t.Run(field, func(t *testing.T) {
			p := prop(t, s, field)
			assertType(t, p, "string")
			if got := p["format"]; got != "date-time" {
				t.Fatalf("%s.format: got %v, want \"date-time\"", field, got)
			}
		})
	}
}

// ---- 7. Combined tags -------------------------------------------------------

type CombinedTags struct {
	Email string `json:"email" desc:"Contact email" pattern:"^[^@]+@[^@]+$" default:"user@example.com" optional:"true"`
	Level int    `json:"level" desc:"Nesting level" min:"1" max:"10" default:"1"`
}

func TestCombinedTags(t *testing.T) {
	s := schema.Generate[CombinedTags]()

	email := prop(t, s, "email")
	assertType(t, email, "string")
	if email["description"] != "Contact email" {
		t.Errorf("email.description: got %v", email["description"])
	}
	if email["pattern"] != "^[^@]+@[^@]+$" {
		t.Errorf("email.pattern: got %v", email["pattern"])
	}
	if email["default"] != "user@example.com" {
		t.Errorf("email.default: got %v", email["default"])
	}
	if hasRequired(s, "email") {
		t.Error("\"email\" must NOT be required due to optional:\"true\"")
	}

	level := prop(t, s, "level")
	assertType(t, level, "integer")
	if level["description"] != "Nesting level" {
		t.Errorf("level.description: got %v", level["description"])
	}
	if level["minimum"] != float64(1) {
		t.Errorf("level.minimum: got %v", level["minimum"])
	}
	if level["maximum"] != float64(10) {
		t.Errorf("level.maximum: got %v", level["maximum"])
	}
	if !reflect.DeepEqual(level["default"], int64(1)) {
		t.Errorf("level.default: got %v (%T), want int64(1)", level["default"], level["default"])
	}
	if !hasRequired(s, "level") {
		t.Error("\"level\" should be required (no optional tag)")
	}
}

// ---- 8. json:"-" skip -------------------------------------------------------

type WithSkipped struct {
	Visible string `json:"visible"`
	Hidden  string `json:"-"`
	// Unexported fields must also be skipped.
	secret string //nolint:unused
}

func TestJSONDashSkip(t *testing.T) {
	s := schema.Generate[WithSkipped]()
	props, ok := s["properties"].(map[string]any)
	if !ok {
		t.Fatal("missing properties")
	}
	if _, found := props["Hidden"]; found {
		t.Error("field with json:\"-\" must be skipped but \"Hidden\" is present under its Go name")
	}
	if _, found := props["-"]; found {
		t.Error("field with json:\"-\" must be skipped but \"-\" key found in properties")
	}
	if _, found := props["visible"]; !found {
		t.Error("\"visible\" field should be present in properties")
	}
	if _, found := props["secret"]; found {
		t.Error("unexported field \"secret\" must be skipped")
	}
}

// ---- 9. Empty struct --------------------------------------------------------

type Empty struct{}

func TestEmptyStruct(t *testing.T) {
	s := schema.Generate[Empty]()
	assertType(t, s, "object")

	props, ok := s["properties"].(map[string]any)
	if !ok {
		t.Fatal("empty struct: missing \"properties\" key")
	}
	if len(props) != 0 {
		t.Fatalf("empty struct: expected 0 properties, got %d", len(props))
	}
	if _, hasReq := s["required"]; hasReq {
		t.Error("empty struct: \"required\" key must be absent when there are no required fields")
	}
}

// ---- 10. Required field logic (all-required by default) ---------------------

type AllRequired struct {
	A string `json:"a"`
	B string `json:"b"`
	C string `json:"c"`
}

func TestAllFieldsRequiredByDefault(t *testing.T) {
	s := schema.Generate[AllRequired]()
	for _, name := range []string{"a", "b", "c"} {
		if !hasRequired(s, name) {
			t.Errorf("field %q should be required by default", name)
		}
	}
}

type AllOptional struct {
	A string `json:"a" optional:"true"`
	B string `json:"b" optional:"true"`
}

func TestAllOptionalProducesNoRequiredKey(t *testing.T) {
	s := schema.Generate[AllOptional]()
	if _, ok := s["required"]; ok {
		t.Error("\"required\" key must be absent when all fields are optional")
	}
}

// ---- 11. GenerateFromType ---------------------------------------------------

func TestGenerateFromType(t *testing.T) {
	type Inner struct {
		Val int `json:"val"`
	}
	t.Run("struct", func(t *testing.T) {
		s := schema.GenerateFromType(reflect.TypeOf(Inner{}))
		assertType(t, s, "object")
		assertType(t, prop(t, s, "val"), "integer")
	})
	t.Run("pointer_to_struct", func(t *testing.T) {
		s := schema.GenerateFromType(reflect.TypeOf((*Inner)(nil)))
		assertType(t, s, "object")
	})
	t.Run("nil_type", func(t *testing.T) {
		s := schema.GenerateFromType(nil)
		assertType(t, s, "object")
	})
	t.Run("string_type", func(t *testing.T) {
		s := schema.GenerateFromType(reflect.TypeOf(""))
		assertType(t, s, "string")
	})
	t.Run("int_type", func(t *testing.T) {
		s := schema.GenerateFromType(reflect.TypeOf(0))
		assertType(t, s, "integer")
	})
}

// ---- 12. json:"name,omitempty" does not affect required --------------------

type OmitemptyStruct struct {
	Name string `json:"name,omitempty"`
	Desc string `json:"desc,omitempty" optional:"true"`
}

func TestOmitemptyDoesNotAffectRequired(t *testing.T) {
	s := schema.Generate[OmitemptyStruct]()

	// "name" has omitempty but no optional:"true" → must be required.
	if !hasRequired(s, "name") {
		t.Error("\"name\" has omitempty but no optional:\"true\" — it must still be required")
	}
	// "desc" has both omitempty and optional:"true" → must NOT be required.
	if hasRequired(s, "desc") {
		t.Error("\"desc\" has optional:\"true\" — it must NOT be required")
	}
}

// ---- 13. Slice of structs ---------------------------------------------------

type Tag struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

type Article struct {
	Title string `json:"title"`
	Tags  []Tag  `json:"tags"`
}

func TestSliceOfStructs(t *testing.T) {
	s := schema.Generate[Article]()
	tagsSchema := prop(t, s, "tags")
	assertType(t, tagsSchema, "array")

	items, ok := tagsSchema["items"].(map[string]any)
	if !ok {
		t.Fatalf("tags.items is not map[string]any; got %T", tagsSchema["items"])
	}
	assertType(t, items, "object")
	assertType(t, prop(t, items, "id"), "integer")
	assertType(t, prop(t, items, "name"), "string")
}

// ---- 14. Map with struct values ---------------------------------------------

type Registry struct {
	Entries map[string]Address `json:"entries"`
}

func TestMapWithStructValues(t *testing.T) {
	s := schema.Generate[Registry]()
	entries := prop(t, s, "entries")
	assertType(t, entries, "object")

	addlProps, ok := entries["additionalProperties"].(map[string]any)
	if !ok {
		t.Fatalf("entries.additionalProperties is not map[string]any; got %T", entries["additionalProperties"])
	}
	assertType(t, addlProps, "object")
	assertType(t, prop(t, addlProps, "street"), "string")
}

// ---- 15. All integer sub-kinds ----------------------------------------------

func TestAllIntegerKinds(t *testing.T) {
	type AllInts struct {
		I   int    `json:"i"`
		I8  int8   `json:"i8"`
		I16 int16  `json:"i16"`
		I32 int32  `json:"i32"`
		I64 int64  `json:"i64"`
		U   uint   `json:"u"`
		U8  uint8  `json:"u8"`
		U16 uint16 `json:"u16"`
		U32 uint32 `json:"u32"`
		U64 uint64 `json:"u64"`
	}
	s := schema.Generate[AllInts]()
	for _, name := range []string{"i", "i8", "i16", "i32", "i64", "u", "u8", "u16", "u32", "u64"} {
		assertType(t, prop(t, s, name), "integer")
	}
}

// ---- 16. Default with bad value falls back to string ------------------------

type BadDefault struct {
	Count int `json:"count" default:"notanumber"`
}

func TestBadDefaultFallsBackToString(t *testing.T) {
	s := schema.Generate[BadDefault]()
	p := prop(t, s, "count")
	got, ok := p["default"]
	if !ok {
		t.Fatal("missing \"default\" key")
	}
	if got != "notanumber" {
		t.Fatalf("bad default should fall back to raw string; got %v (%T)", got, got)
	}
}

// ---- 17. Circular struct reference (must not stack-overflow) ----------------

// SelfRef is a struct that contains a pointer to itself, the classic circular
// reference that previously caused an infinite-recursion stack overflow in
// buildSchema.
type SelfRef struct {
	Value    int      `json:"value"`
	Children *SelfRef `json:"children" optional:"true"`
}

func TestCircularStructReference_NoStackOverflow(t *testing.T) {
	// This must complete without panicking or stack-overflowing.
	s := schema.Generate[SelfRef]()
	assertType(t, s, "object")

	// The top-level fields must be present.
	assertType(t, prop(t, s, "value"), "integer")

	// The self-referential field must be emitted as a plain object (the
	// recursion guard fires before the type can be fully expanded again).
	children := prop(t, s, "children")
	assertType(t, children, "object")
}

// MutualA and MutualB form a mutually recursive cycle.
type MutualA struct {
	Name string   `json:"name"`
	B    *MutualB `json:"b" optional:"true"`
}

type MutualB struct {
	ID int      `json:"id"`
	A  *MutualA `json:"a" optional:"true"`
}

func TestMutuallyCircularStructs_NoStackOverflow(t *testing.T) {
	s := schema.Generate[MutualA]()
	assertType(t, s, "object")
	assertType(t, prop(t, s, "name"), "string")
	// B's schema must be present but its recursive A field is cut off.
	bSchema := prop(t, s, "b")
	assertType(t, bSchema, "object")
}

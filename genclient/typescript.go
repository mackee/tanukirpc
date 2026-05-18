package genclient

import (
	"bytes"
	"embed"
	"errors"
	"fmt"
	"go/token"
	"go/types"
	"io"
	"os"
	"reflect"
	"strings"
	"text/template"

	"golang.org/x/tools/go/analysis"
)

// jsonMarshalerWhitelist maps named types whose custom MarshalJSON is known
// to emit a specific TypeScript primitive shape to the TS type they should
// render as. gentypescript otherwise rejects types with a custom
// MarshalJSON because it cannot infer the wire format statically — users
// must supply a tstype:"..." tag on the field.
//
// Add entries here only when the named type's MarshalJSON contract is
// well-defined (time.Time always emits an RFC3339 string, etc.).
var jsonMarshalerWhitelist = map[string]typeScriptClientGeneratorLiteralType{
	"time.Time": "string",
}

// unwrapFieldPointer reports whether t is a pointer type (directly or via a
// defined / aliased name) and returns the pointee. It is used by toFields to
// decide whether a field is optional before delegating the rest of type
// resolution to typeInfo. Handles:
//
//	*T          → (T, true)
//	type A *T   → (T, true)   // named-pointer underlying
//	type A = *T → (T, true)   // alias to *T
//
// Other types — including named non-pointer types like time.Time — are
// returned unchanged so the whitelist / MarshalJSON detection in typeInfo
// still gets the chance to act on them.
func unwrapFieldPointer(t types.Type) (types.Type, bool) {
	if pt, ok := types.Unalias(t).(*types.Pointer); ok {
		return pt.Elem(), true
	}
	if nt, ok := t.(*types.Named); ok {
		if pt, ok := types.Unalias(nt.Underlying()).(*types.Pointer); ok {
			return pt.Elem(), true
		}
	}
	return t, false
}

// tryRenderJSONMarshalerWhitelist returns the literal TS field to use for a
// type whose custom MarshalJSON is in jsonMarshalerWhitelist. A pointer to a
// whitelisted type counts (the pointer dereference is invisible on the wire).
// Returns ok=false when the type is not in the whitelist.
func tryRenderJSONMarshalerWhitelist(t types.Type) (typeScriptClientGeneratorField, bool) {
	if pt, ok := t.(*types.Pointer); ok {
		t = pt.Elem()
	}
	nt, ok := t.(*types.Named)
	if !ok {
		return nil, false
	}
	lit, ok := jsonMarshalerWhitelist[nt.String()]
	if !ok {
		return nil, false
	}
	return lit, true
}

// hasCustomJSONMarshaler reports whether t (or *t) declares a MarshalJSON
// method matching the encoding/json contract — i.e. `MarshalJSON() ([]byte,
// error)`. gentypescript uses this to reject types whose wire format it
// cannot infer statically; users override these with a tstype:"..." tag.
func hasCustomJSONMarshaler(t types.Type) bool {
	if pt, ok := t.(*types.Pointer); ok {
		t = pt.Elem()
	}
	obj, _, _ := types.LookupFieldOrMethod(t, true, nil, "MarshalJSON")
	fn, ok := obj.(*types.Func)
	if !ok {
		return false
	}
	sig, ok := fn.Type().(*types.Signature)
	if !ok {
		return false
	}
	if sig.Params().Len() != 0 || sig.Results().Len() != 2 {
		return false
	}
	sl, ok := sig.Results().At(0).Type().(*types.Slice)
	if !ok {
		return false
	}
	b, ok := sl.Elem().(*types.Basic)
	if !ok || b.Kind() != types.Byte {
		return false
	}
	errType := types.Universe.Lookup("error")
	if errType == nil {
		return false
	}
	return types.Identical(sig.Results().At(1).Type(), errType.Type())
}

//go:embed typescriptclient.tmpl
var typeScriptClientTemplate embed.FS

var TypeScriptClientGenerator = &analysis.Analyzer{
	Name: "gentypescript",
	Doc:  "generate TypeScript client code",
	Run:  generateTypeScriptClient,
	Requires: []*analysis.Analyzer{
		Analyzer,
	},
	ResultType: reflect.TypeFor[*bytes.Buffer](),
}

var typeScriptClientOutPath string

func init() {
	TypeScriptClientGenerator.Flags.StringVar(&typeScriptClientOutPath, "out", "", "output file path")
}

func generateTypeScriptClient(pass *analysis.Pass) (any, error) {
	result := pass.ResultOf[Analyzer].(*AnalyzerResult)
	if len(result.MultipleAnalyzeTargetPositions) > 0 {
		for _, pos := range result.MultipleAnalyzeTargetPositions {
			pass.Reportf(pos,
				"gentypescript: AnalyzeTarget must be called at most once per package; the generated client cannot represent more than one router's routes and error body")
		}
		return &bytes.Buffer{}, nil
	}
	for _, pos := range result.UnresolvedOptions {
		pass.Reportf(pos,
			"gentypescript: could not statically determine whether this RouterOption configures the error response body; the generated TypeScript ErrorResponse type may not match runtime behavior")
	}
	reportErrorBodyTypeWarnings(pass, result.ErrorBody, result.AnalyzeTargetCallPos)
	if reportErrorBodyFatalErrors(pass, result.ErrorBody, result.AnalyzeTargetCallPos) {
		return &bytes.Buffer{}, nil
	}
	if reportErrorBodyNestedFatalErrors(pass, result.ErrorBody, result.AnalyzeTargetCallPos) {
		return &bytes.Buffer{}, nil
	}
	if len(result.RoutePaths) == 0 {
		return &bytes.Buffer{}, nil
	}

	gen, err := newTypeScriptClientGenerator()
	if err != nil {
		return nil, fmt.Errorf("failed to create TypeScript client generator: %w", err)
	}
	if err := gen.generate(result.RoutePaths, result.ErrorBody); err != nil {
		if errors.Is(err, errNoErrorBodyDiscriminator) {
			pos := errorBodyDeclarationPos(result.ErrorBody, result.AnalyzeTargetCallPos)
			pass.Reportf(pos,
				"gentypescript: error body type %s has no required field that can discriminate it from a success response at runtime. Every required field is either marked omitempty, has a nilable container type (slice / map), or is otherwise unsuitable for a typeof check. Add at least one non-omitempty primitive (string / number / boolean) or non-pointer nested struct field so the generated `isErrorResponse` predicate can recognize error responses.",
				result.ErrorBody.String())
			return &bytes.Buffer{}, nil
		}
		return nil, fmt.Errorf("failed to generate TypeScript client code: %w", err)
	}
	if typeScriptClientOutPath != "" {
		f, err := os.Create(typeScriptClientOutPath)
		if err != nil {
			return nil, fmt.Errorf("failed to create output file: %w", err)
		}
		if _, err := io.Copy(f, gen.rw); err != nil {
			return nil, fmt.Errorf("failed to write output file: %w", err)
		}
	}

	return gen.rw, nil
}

// reportErrorBodyFatalErrors inspects the registered error body Go type and
// reports diagnostics that gentypescript treats as fatal. Returns true when a
// fatal error was emitted and the caller must skip client generation.
//
// Currently fatal:
//   - The error body has no json-tagged fields that gentypescript can render
//     (empty struct, struct whose fields are all skipped, or struct that
//     only contains embedded fields). Without a fatal error the generated
//     `ErrorResponse` collapses to `error: undefined`, while encoding/json
//     emits `{"error":{}}` at runtime — a shape clients cannot satisfy.
func reportErrorBodyFatalErrors(pass *analysis.Pass, tt types.Type, fallbackPos token.Pos) bool {
	if tt == nil {
		return false
	}
	inner := tt
	if pt, ok := inner.(*types.Pointer); ok {
		inner = pt.Elem()
	}
	st := underlyingStruct(inner)
	if st == nil {
		return false
	}
	if hasRenderableJSONField(st) {
		return false
	}
	pos := token.NoPos
	if nt, ok := inner.(*types.Named); ok && nt.Obj() != nil {
		pos = nt.Obj().Pos()
	}
	if !pos.IsValid() {
		pos = fallbackPos
	}
	if structHasOnlyEmbeddedFields(st) {
		pass.Reportf(pos,
			"gentypescript: error body type %s contains only embedded fields; encoding/json would flatten them into %q at runtime, but gentypescript does not represent embedded fields in the generated TypeScript ErrorResponse. Replace the embedded fields with explicit json-tagged fields.",
			tt.String(), `{"error":{...}}`)
		return true
	}
	pass.Reportf(pos,
		"gentypescript: error body type %s has no json-tagged fields that gentypescript can render; encoding/json would emit %q at runtime but the generated TypeScript ErrorResponse type would collapse to an unusable shape. Add at least one explicit json-tagged field.",
		tt.String(), `{"error":{}}`)
	return true
}

// reportErrorBodyNestedFatalErrors walks the error body type and rejects
// types whose wire shape cannot be inferred from Go type information alone:
//
//   - Any type implementing encoding/json.Marshaler that gentypescript does
//     not have a hard-coded mapping for (the jsonMarshalerWhitelist —
//     currently time.Time). The custom MarshalJSON could emit any shape, so
//     users must declare the TypeScript representation with a tstype tag on
//     the containing field.
//   - Interface-typed fields (including `any`). encoding/json picks the
//     concrete type at runtime, so the static shape is undefined.
//
// Walks through pointers, slices, maps (values), and nested structs. Stops
// at any field that already carries a tstype:"..." tag (the user has taken
// responsibility for the shape there) and at whitelisted types. Returns
// true when at least one diagnostic was emitted.
func reportErrorBodyNestedFatalErrors(pass *analysis.Pass, tt types.Type, fallbackPos token.Pos) bool {
	if tt == nil {
		return false
	}
	w := &errorBodyFatalWalker{pass: pass, fallbackPos: fallbackPos, visited: map[types.Type]bool{}}
	w.walk(tt, fallbackPos)
	return w.emitted
}

type errorBodyFatalWalker struct {
	pass        *analysis.Pass
	fallbackPos token.Pos
	visited     map[types.Type]bool
	emitted     bool
}

func (w *errorBodyFatalWalker) walk(tt types.Type, pos token.Pos) {
	if tt == nil || w.visited[tt] {
		return
	}
	w.visited[tt] = true
	tt = types.Unalias(tt)
	if pt, ok := tt.(*types.Pointer); ok {
		tt = types.Unalias(pt.Elem())
	}
	if _, ok := tryRenderJSONMarshalerWhitelist(tt); ok {
		return
	}
	if hasCustomJSONMarshaler(tt) {
		reportPos := pos
		if !reportPos.IsValid() {
			reportPos = w.fallbackPos
		}
		w.pass.Reportf(reportPos,
			"gentypescript: error body field has type %s with a custom MarshalJSON method; gentypescript cannot infer its wire shape from the Go type. Declare the field with a `tstype:\"...\"` struct tag to specify the TypeScript type, or add a hard-coded mapping for this type to the gentypescript whitelist.",
			tt.String())
		w.emitted = true
		return
	}
	switch u := tt.(type) {
	case *types.Interface:
		reportPos := pos
		if !reportPos.IsValid() {
			reportPos = w.fallbackPos
		}
		w.pass.Reportf(reportPos,
			"gentypescript: error body field has interface type %s; encoding/json picks the concrete type at runtime so gentypescript cannot infer a static wire shape. Declare the field with a `tstype:\"...\"` struct tag to specify the TypeScript type.",
			tt.String())
		w.emitted = true
	case *types.Slice:
		w.walk(u.Elem(), pos)
	case *types.Map:
		w.walk(u.Elem(), pos)
	case *types.Named:
		w.walk(u.Underlying(), pos)
	case *types.Struct:
		for i := 0; i < u.NumFields(); i++ {
			f := u.Field(i)
			tag := reflect.StructTag(u.Tag(i))
			// Fields with json:"-" or no json tag are not part of the wire
			// shape; users can keep arbitrary types there.
			jv := tag.Get("json")
			if jv == "" {
				continue
			}
			if name := strings.Split(jv, ",")[0]; name == "-" {
				continue
			}
			// Unexported fields are dropped by encoding/json regardless.
			if !f.Exported() {
				continue
			}
			// A tstype tag means the user has declared the TS shape
			// explicitly — gentypescript does not need to introspect.
			if tag.Get("tstype") != "" {
				continue
			}
			w.walk(f.Type(), f.Pos())
		}
	}
}

// structHasOnlyEmbeddedFields reports whether every field of st is embedded.
// A struct with zero fields returns false (handled by the generic empty-body
// message instead).
func structHasOnlyEmbeddedFields(st *types.Struct) bool {
	if st.NumFields() == 0 {
		return false
	}
	for i := 0; i < st.NumFields(); i++ {
		if !st.Field(i).Embedded() {
			return false
		}
	}
	return true
}

// hasRenderableJSONField reports whether st has at least one direct field
// that gentypescript would emit into the generated TypeScript ErrorResponse.
// Embedded fields are not rendered, so they are skipped here. Unexported
// fields are also skipped because encoding/json drops them at runtime.
func hasRenderableJSONField(st *types.Struct) bool {
	for i := 0; i < st.NumFields(); i++ {
		f := st.Field(i)
		if f.Embedded() {
			continue
		}
		if !f.Exported() {
			continue
		}
		tag := reflect.StructTag(st.Tag(i))
		v := tag.Get("json")
		if v == "" {
			continue
		}
		name := strings.Split(v, ",")[0]
		if name == "" || name == "-" {
			continue
		}
		return true
	}
	return false
}

// reportErrorBodyTypeWarnings inspects the registered error body Go type and
// emits diagnostics for shapes that would silently misrepresent runtime
// behavior in the generated TypeScript:
//   - Pointer E: encoding/json can emit null, but the generated ErrorResponse
//     type is not nullable.
//   - Exported struct fields without a json tag: encoding/json names them by
//     Go field name, but toFields skips them so the generated TS omits them.
//   - Embedded struct fields: encoding/json flattens them into the parent
//     object, but gentypescript does not, so the generated TS omits the
//     flattened fields. Use explicit json-tagged fields instead.
func reportErrorBodyTypeWarnings(pass *analysis.Pass, tt types.Type, fallbackPos token.Pos) {
	if tt == nil {
		return
	}
	if pt, ok := tt.(*types.Pointer); ok {
		pos := token.NoPos
		if nt, ok := pt.Elem().(*types.Named); ok && nt.Obj() != nil {
			pos = nt.Obj().Pos()
		}
		if !pos.IsValid() {
			pos = fallbackPos
		}
		pass.Reportf(pos,
			"gentypescript: error body type %s is a pointer; runtime encoding/json may emit {\"error\": null}, but the generated TypeScript ErrorResponse type is not nullable. Use a struct value type instead.",
			tt.String())
		tt = pt.Elem()
	}
	st := underlyingStruct(tt)
	if st == nil {
		return
	}
	reportUntaggedFields(pass, st)
}

// reportUntaggedFields warns at every direct exported field that lacks a json
// tag, and at every embedded field that encoding/json would flatten (i.e.
// embedded fields without `json:"-"`). gentypescript does not represent
// embedded fields in the generated TypeScript ErrorResponse.
func reportUntaggedFields(pass *analysis.Pass, st *types.Struct) {
	for i := 0; i < st.NumFields(); i++ {
		f := st.Field(i)
		tag := reflect.StructTag(st.Tag(i))
		if f.Embedded() {
			if name := strings.Split(tag.Get("json"), ",")[0]; name == "-" {
				continue
			}
			pass.Reportf(f.Pos(),
				"gentypescript: error body has an embedded field %q; encoding/json flattens its contents at runtime, but the generated TypeScript ErrorResponse omits them. Replace it with explicit json-tagged fields.",
				f.Name())
			continue
		}
		if !f.Exported() {
			if name := strings.Split(tag.Get("json"), ",")[0]; name != "" && name != "-" {
				pass.Reportf(f.Pos(),
					"gentypescript: error body field %q is unexported but has a json tag; encoding/json drops unexported fields at runtime, so the generated TypeScript ErrorResponse will not match the wire format. Export the field (capitalize the name) to include it.",
					f.Name())
			}
			continue
		}
		if tag.Get("json") != "" {
			continue
		}
		pass.Reportf(f.Pos(),
			"gentypescript: error body field %q has no json tag; encoding/json will emit it as %q at runtime, but the generated TypeScript ErrorResponse will omit it",
			f.Name(), f.Name())
	}
}

// underlyingStruct returns the *types.Struct underlying tt, peeling pointer
// and named-type wrappers. Returns nil if tt does not bottom out at a struct.
func underlyingStruct(tt types.Type) *types.Struct {
	for {
		switch u := tt.(type) {
		case *types.Pointer:
			tt = u.Elem()
		case *types.Named:
			tt = u.Underlying()
		case *types.Struct:
			return u
		default:
			return nil
		}
	}
}

type typeScriptClientGenerator struct {
	rw   *bytes.Buffer
	tmpl *template.Template
}

func newTypeScriptClientGenerator() (*typeScriptClientGenerator, error) {
	tmpl, err := template.ParseFS(typeScriptClientTemplate, "typescriptclient.tmpl")
	if err != nil {
		return nil, fmt.Errorf("failed to parse template: %w", err)
	}

	buf := &bytes.Buffer{}
	return &typeScriptClientGenerator{
		rw:   buf,
		tmpl: tmpl,
	}, nil
}

func (t *typeScriptClientGenerator) generate(routes []RoutePath, errorBody types.Type) error {
	templateArgs := make(typeScriptClientGeneratorTemplateArgs, 0, len(routes))
	for _, r := range routes {
		h := r.Handler()
		mp := &typeScriptClientGeneratorTemplateArgsMethodPath{
			Method: r.Method(),
			Path:   r.Path(),
		}

		// query of request
		if of, err := t.typeInfo(h.Req(), "query"); err != nil {
			return fmt.Errorf("failed to generate request type of route %s %s: %w", r.Method(), r.Path(), err)
		} else {
			mp.Query = topLevelOrVoid(of)
		}

		// json of request
		if of, err := t.typeInfo(h.Req(), "json"); err != nil {
			return fmt.Errorf("failed to generate request type of route %s %s: %w", r.Method(), r.Path(), err)
		} else {
			mp.Request = topLevelOrVoid(of)
		}

		// json of response
		if of, err := t.typeInfo(h.Res(), "json"); err != nil {
			return fmt.Errorf("failed to generate response type of route %s %s: %w", r.Method(), r.Path(), err)
		} else {
			mp.Response = topLevelOrVoid(of)
		}

		templateArgs = append(templateArgs, mp)
	}

	templateData := &typeScriptClientGeneratorTemplateData{Routes: templateArgs}
	if errorBody != nil {
		eb, err := t.buildErrorBody(errorBody)
		if err != nil {
			return fmt.Errorf("failed to generate error body type: %w", err)
		}
		templateData.ErrorBody = eb
	}

	if err := t.tmpl.Execute(t.rw, templateData); err != nil {
		return fmt.Errorf("failed to execute template: %w", err)
	}
	return nil
}

// buildErrorBody converts the analyzed ErrorBody Go type into the data passed
// to the TypeScript template: a rendered object field and the list of json
// fields that should drive the isErrorResponse predicate. Primitive fields
// emit a typeof check; struct/map/slice fields emit an object-presence check
// so error bodies whose required fields are non-primitives (for example
// {errors: Record<string,string>}) can still be discriminated at runtime.
func (t *typeScriptClientGenerator) buildErrorBody(tt types.Type) (*typeScriptClientGeneratorErrorBody, error) {
	field, err := t.typeInfo(tt, "json")
	if err != nil {
		return nil, err
	}
	obj, ok := field.(*typeScriptClientGeneratorObjectField)
	if !ok {
		return nil, fmt.Errorf("error body type %s did not render to an object", tt.String())
	}
	required := make([]typeScriptClientGeneratorErrorBodyField, 0, len(obj.fields))
	for _, f := range obj.fields {
		gf, ok := f.(*typeScriptClientGeneratorGenericField)
		if !ok {
			continue
		}
		if gf.isOption {
			continue
		}
		pred, ok := errorBodyFieldPredicate(gf)
		if !ok {
			continue
		}
		required = append(required, typeScriptClientGeneratorErrorBodyField{
			Predicate: pred,
		})
	}
	if len(required) == 0 {
		return nil, errNoErrorBodyDiscriminator
	}
	return &typeScriptClientGeneratorErrorBody{
		Field:          obj,
		RequiredFields: required,
	}, nil
}

// topLevelOrVoid collapses an empty object (a struct with no renderable
// fields) to the Void marker. Used at top-level call sites — handler
// request / response — where `struct{}` should surface in client.ts as
// `Request: undefined` / `Response: undefined` rather than `{}`. Nested
// occurrences of empty structs (struct fields, map values, slice elements)
// must keep the `{}` rendering to match encoding/json's wire format.
func topLevelOrVoid(f typeScriptClientGeneratorField) typeScriptClientGeneratorField {
	obj, ok := f.(*typeScriptClientGeneratorObjectField)
	if !ok {
		return f
	}
	if len(obj.fields) == 0 {
		return &typeScriptClientGeneratorVoidField{}
	}
	return obj
}

// errNoErrorBodyDiscriminator signals that the error body type has no
// required field gentypescript can use to discriminate it from a success
// response. The caller converts this into a pass-level diagnostic.
var errNoErrorBodyDiscriminator = errors.New("error body has no required discriminator field")

// errorBodyDeclarationPos returns the source position of the error body type
// declaration when available, falling back to the AnalyzeTarget call site so
// the diagnostic is always anchored at a meaningful location.
func errorBodyDeclarationPos(tt types.Type, fallback token.Pos) token.Pos {
	if tt == nil {
		return fallback
	}
	inner := tt
	if pt, ok := inner.(*types.Pointer); ok {
		inner = pt.Elem()
	}
	if nt, ok := inner.(*types.Named); ok && nt.Obj() != nil {
		if pos := nt.Obj().Pos(); pos.IsValid() {
			return pos
		}
	}
	return fallback
}

// tsPropertyAccessor renders a property access expression on a TypeScript
// variable. Dot notation is preferred when the property name is a valid JS
// identifier (avoids biome's useLiteralKeys / eslint's dot-notation lint
// rules); bracket notation is the fallback for names with characters that
// dot syntax cannot reach.
func tsPropertyAccessor(receiver, name string) string {
	if isJSIdentifier(name) {
		return receiver + "." + name
	}
	return fmt.Sprintf("%s[%q]", receiver, name)
}

func isJSIdentifier(s string) bool {
	if s == "" {
		return false
	}
	for i, r := range s {
		switch {
		case r == '_' || r == '$':
		case 'a' <= r && r <= 'z':
		case 'A' <= r && r <= 'Z':
		case i > 0 && '0' <= r && r <= '9':
		default:
			return false
		}
	}
	return true
}

// errorBodyFieldPredicate returns a TypeScript expression that checks for the
// runtime presence of the field on a `Record<string, unknown>` variable named
// `e`. Returns ok=false for fields that cannot reliably discriminate the
// error body shape at runtime:
//
//   - Slice / map / pointer-to-anything fields: encoding/json emits `null`
//     when the underlying value is nil and the field is not omitempty, which
//     would fail the presence check and cause a valid error response to be
//     classified as a success.
//   - Inertia / void / unknown wrappers: their TypeScript shape is not a
//     simple presence-checkable primitive.
//
// Struct *value* fields are kept because encoding/json always emits an
// object (never null) for them.
func errorBodyFieldPredicate(gf *typeScriptClientGeneratorGenericField) (string, bool) {
	if gf.isSlice {
		return "", false
	}
	accessor := tsPropertyAccessor("e", gf.name)
	switch td := gf.typedef.(type) {
	case typeScriptClientGeneratorLiteralType:
		kind := string(td)
		if kind != "string" && kind != "number" && kind != "boolean" {
			return "", false
		}
		return fmt.Sprintf("typeof %s === %q", accessor, kind), true
	case *typeScriptClientGeneratorObjectField:
		// Non-pointer struct value: always `{...}` on the wire, never null.
		// Pointer-to-struct surfaces here too if isOption=false, which the
		// caller already filters out via gf.isOption, so we only see safely-
		// always-object fields.
		return fmt.Sprintf("typeof %s === \"object\" && %s !== null", accessor, accessor), true
	default:
		return "", false
	}
}

type typeScriptClientGeneratorField interface {
	RenderRequest(prefix string) string
	RenderResponse(prefix string) string
}

type typeScriptClientGeneratorObjectField struct {
	fields []typeScriptClientGeneratorField
}

func (t *typeScriptClientGeneratorObjectField) RenderRequest(prefix string) string {
	if len(t.fields) == 0 {
		return "{}"
	}
	var ret strings.Builder
	ret.WriteString("{\n")
	for _, field := range t.fields {
		ret.WriteString(field.RenderRequest(prefix+"  ") + "\n")
	}
	ret.WriteString(prefix + "}")
	return ret.String()
}

func (t *typeScriptClientGeneratorObjectField) RenderResponse(prefix string) string {
	if len(t.fields) == 0 {
		return "{}"
	}
	var ret strings.Builder
	ret.WriteString("{\n")
	for _, field := range t.fields {
		ret.WriteString(field.RenderResponse(prefix+"  ") + "\n")
	}
	ret.WriteString(prefix + "}")
	return ret.String()
}

type typeScriptClientGeneratorGenericField struct {
	name       string
	typedef    typeScriptClientGeneratorField
	isSlice    bool
	isRequired bool
	isOption   bool
}

func (t *typeScriptClientGeneratorGenericField) sliceSuffix() string {
	if t.isSlice {
		return "[]"
	}
	return ""
}

func (t *typeScriptClientGeneratorGenericField) isRequiredOpRequest() string {
	if t.isRequired {
		return ""
	}
	return "?"
}

func (t *typeScriptClientGeneratorGenericField) isRequiredOpResponse() string {
	if !t.isRequired && t.isOption {
		return "?"
	}
	return ""
}

func (t *typeScriptClientGeneratorGenericField) RenderRequest(prefix string) string {
	return fmt.Sprintf("%s%s%s: %s%s;", prefix, t.name, t.isRequiredOpRequest(), t.typedef.RenderRequest(prefix), t.sliceSuffix())
}

func (t *typeScriptClientGeneratorGenericField) RenderResponse(prefix string) string {
	return fmt.Sprintf("%s%s%s: %s%s;", prefix, t.name, t.isRequiredOpResponse(), t.typedef.RenderResponse(prefix), t.sliceSuffix())
}

type typeScriptClientGeneratorLiteralType string

func (t typeScriptClientGeneratorLiteralType) RenderRequest(prefix string) string {
	return string(t)
}

func (t typeScriptClientGeneratorLiteralType) RenderResponse(prefix string) string {
	return string(t)
}

// typeScriptClientGeneratorSliceField renders a Go []T as a TypeScript T[].
// Used by typeInfo when a slice appears nested inside another structure
// (map values, inertia props). For top-level struct fields toFields still
// records the slice via the isSlice flag so the field-level `?:` and []
// suffixes stay where they are.
type typeScriptClientGeneratorSliceField struct {
	elem typeScriptClientGeneratorField
}

func (t *typeScriptClientGeneratorSliceField) RenderRequest(prefix string) string {
	return fmt.Sprintf("%s[]", t.elem.RenderRequest(prefix))
}

func (t *typeScriptClientGeneratorSliceField) RenderResponse(prefix string) string {
	return fmt.Sprintf("%s[]", t.elem.RenderResponse(prefix))
}

// typeScriptClientGeneratorMapField renders a Go map[string]V as a TypeScript
// Record<string, V>. Non-string key maps are not supported (json's wire shape
// requires string keys anyway).
type typeScriptClientGeneratorMapField struct {
	value typeScriptClientGeneratorField
}

func (t *typeScriptClientGeneratorMapField) RenderRequest(prefix string) string {
	return fmt.Sprintf("Record<string, %s>", t.value.RenderRequest(prefix))
}

func (t *typeScriptClientGeneratorMapField) RenderResponse(prefix string) string {
	return fmt.Sprintf("Record<string, %s>", t.value.RenderResponse(prefix))
}

type typeScriptClientGeneratorVoidField struct{}

func (t *typeScriptClientGeneratorVoidField) RenderRequest(prefix string) string {
	return "undefined"
}

func (t *typeScriptClientGeneratorVoidField) RenderResponse(prefix string) string {
	return "undefined"
}

type typeScriptClientGeneratorInertiaPageField struct {
	props typeScriptClientGeneratorField
}

func (t *typeScriptClientGeneratorInertiaPageField) RenderRequest(prefix string) string {
	return t.RenderResponse(prefix)
}

func (t *typeScriptClientGeneratorInertiaPageField) RenderResponse(prefix string) string {
	props := t.props.RenderResponse(prefix + "  ")
	if _, ok := t.props.(*typeScriptClientGeneratorVoidField); ok {
		props = "{}"
	}
	return fmt.Sprintf(`{
%s  component: string;
%s  props: %s;
%s  url: string;
%s  version?: unknown;
%s  encryptHistory?: boolean;
%s  clearHistory?: boolean;
%s  preserveFragment?: boolean;
%s}`, prefix, prefix, props, prefix, prefix, prefix, prefix, prefix, prefix)
}

// typeInfo is the central type → TypeScript field resolver. Every recursive
// path (map value, slice element, inertia props, top-level handler request /
// response) flows through this function so the whitelist and the custom-
// MarshalJSON rejection are applied consistently.
//
// Resolution order:
//  1. Peel a single pointer wrapper (nullability is not modeled in the
//     generated TS type; this matches the pre-existing field-level behavior).
//  2. Whitelist match (time.Time → "string", etc.) — short-circuit.
//  3. Custom MarshalJSON detected → return an error suggesting tstype on
//     the containing field.
//  4. Inertia Page[T] → render as the codec-specific page envelope.
//  5. Slice / Map → wrap and recurse on the element type.
//  6. Interface → return an error (no static wire shape).
//  7. Named → unwrap to underlying and recurse.
//  8. Struct → toFields, then return either the object or a void marker.
//  9. Basic → primitive literal.
func (t *typeScriptClientGenerator) typeInfo(tt types.Type, tagFilter string) (typeScriptClientGeneratorField, error) {
	tt = types.Unalias(tt)
	if pt, ok := tt.(*types.Pointer); ok {
		tt = types.Unalias(pt.Elem())
	}
	if field, ok := tryRenderJSONMarshalerWhitelist(tt); ok {
		return field, nil
	}
	if hasCustomJSONMarshaler(tt) {
		return nil, fmt.Errorf(
			"type %s has a custom MarshalJSON method; gentypescript cannot infer its wire shape statically — declare the field with a `tstype:\"...\"` struct tag to specify the TypeScript type",
			tt.String())
	}
	if field, ok, err := t.inertiaPageTypeInfo(tt, tagFilter); ok || err != nil {
		return field, err
	}
	switch u := tt.(type) {
	case *types.Slice:
		elem, err := t.typeInfo(u.Elem(), tagFilter)
		if err != nil {
			return nil, fmt.Errorf("slice element of %s: %w", tt.String(), err)
		}
		return &typeScriptClientGeneratorSliceField{elem: elem}, nil
	case *types.Map:
		return t.mapTypeInfo(u, tagFilter)
	case *types.Interface:
		return nil, fmt.Errorf(
			"interface type %s has no statically inferable wire shape; declare the field with a `tstype:\"...\"` struct tag to specify the TypeScript type",
			tt.String())
	case *types.Named:
		return t.typeInfo(u.Underlying(), tagFilter)
	case *types.Struct:
		// Always return an ObjectField, even for an empty struct or a
		// struct whose fields are all dropped (no json tag, unexported,
		// embedded-only, etc.). encoding/json emits `{}` on the wire for
		// such values, so the nested TS type must reflect that. The
		// top-level Void substitution (handler request / response that
		// is `struct{}`) is done by topLevelOrVoid at the entry points.
		fields, err := t.toFields(u, tagFilter)
		if err != nil {
			return nil, fmt.Errorf("failed to convert fields: %w", err)
		}
		return &typeScriptClientGeneratorObjectField{fields: fields}, nil
	case *types.Basic:
		typename, err := t.typeNameByBasicLit(u)
		if err != nil {
			return nil, fmt.Errorf("failed to convert basic type: %w", err)
		}
		return typeScriptClientGeneratorLiteralType(typename), nil
	default:
		return nil, fmt.Errorf("unsupported type: %s", tt.String())
	}
}

func (t *typeScriptClientGenerator) inertiaPageTypeInfo(tt types.Type, tagFilter string) (typeScriptClientGeneratorField, bool, error) {
	if tagFilter != "json" {
		return nil, false, nil
	}
	nt, ok := tt.(*types.Named)
	if !ok {
		return nil, false, nil
	}
	obj := nt.Obj()
	if obj == nil || obj.Pkg() == nil || obj.Pkg().Path() != "github.com/mackee/tanukirpc/codec/inertiajs" || obj.Name() != "Page" {
		return nil, false, nil
	}
	args := nt.TypeArgs()
	if args == nil || args.Len() != 1 {
		return nil, true, fmt.Errorf("unsupported inertia page type: %s", tt.String())
	}
	props, err := t.typeInfo(args.At(0), tagFilter)
	if err != nil {
		return nil, true, fmt.Errorf("failed to generate inertia props type: %w", err)
	}
	return &typeScriptClientGeneratorInertiaPageField{props: props}, true, nil
}

func (t *typeScriptClientGenerator) toFields(tt *types.Struct, filterTag string) ([]typeScriptClientGeneratorField, error) {
	fields := make([]typeScriptClientGeneratorField, 0, tt.NumFields())
	for i := 0; i < tt.NumFields(); i++ {
		f := tt.Field(i)
		tag := reflect.StructTag(tt.Tag(i))

		tagValue := tag.Get(filterTag)
		if tagValue == "" {
			continue
		}
		tagFieldName := strings.Split(tagValue, ",")[0]
		if tagFieldName == "-" {
			continue
		}
		// Unexported fields are dropped by encoding/json at runtime even when
		// they carry a tag, so the generated TypeScript must not include them
		// either. The error-body analyzer surfaces this as a warning so users
		// notice the mismatch with their declared tag.
		if !f.Exported() {
			continue
		}
		fieldName := tagFieldName

		var required bool
		validateTag := tag.Get("validate")
		if tag.Get("required") == "true" ||
			strings.HasPrefix(validateTag, "required,") ||
			strings.HasSuffix(validateTag, ",required") ||
			strings.Contains(validateTag, ",required,") ||
			validateTag == "required" {
			required = true
		}

		var option bool
		if strings.HasPrefix(tagValue, "omitempty,") ||
			strings.HasSuffix(tagValue, ",omitempty") ||
			strings.Contains(tagValue, ",omitempty,") ||
			tagValue == "omitempty" {
			option = true
		}
		if jsType := tag.Get("tstype"); jsType != "" {
			fields = append(fields, &typeScriptClientGeneratorGenericField{
				name:       fieldName,
				typedef:    typeScriptClientGeneratorLiteralType(jsType),
				isSlice:    false,
				isRequired: required,
				isOption:   option,
			})
			continue
		}

		// Peel a single field-level pointer (sets the optional `?` suffix
		// in the rendered TS field) and a single field-level slice (sets
		// the `[]` suffix). The remaining type is handed to typeInfo,
		// which enforces the whitelist / MarshalJSON / interface rules.
		//
		// unwrapFieldPointer also recognises pointer types hidden behind a
		// defined / aliased name (`type MaybeFoo *Foo`, `type MaybeFoo =
		// *Foo`). Without this the pointer is silently peeled inside
		// typeInfo with no chance to mark the field optional, and the
		// generated TypeScript would lose nilability.
		ft := f.Type()
		if elem, isPtr := unwrapFieldPointer(ft); isPtr {
			option = true
			ft = elem
		}
		isSlice := false
		if st, ok := ft.(*types.Slice); ok {
			ft = st.Elem()
			isSlice = true
		}
		typedef, err := t.typeInfo(ft, filterTag)
		if err != nil {
			return nil, fmt.Errorf("field %q: %w", fieldName, err)
		}

		// Embedded struct fields are flattened into the parent object so
		// the generated TS mirrors encoding/json's behavior. Only applies
		// when the embedded type resolved to an object (anything else,
		// e.g. an embedded named primitive alias, falls through to a
		// regular field — that case is rare and would be flagged by the
		// embedded-field warning anyway).
		if f.Embedded() && !isSlice {
			if obj, ok := typedef.(*typeScriptClientGeneratorObjectField); ok {
				fields = append(fields, obj.fields...)
				continue
			}
		}

		fields = append(fields, &typeScriptClientGeneratorGenericField{
			name:       fieldName,
			typedef:    typedef,
			isSlice:    isSlice,
			isRequired: required,
			isOption:   option,
		})
	}
	return fields, nil
}

// mapTypeInfo renders a Go map[string]V as a Record<string, V> field.
// Maps with a non-string key are rejected because encoding/json requires the
// key type to be encoded as a string at the wire level.
func (t *typeScriptClientGenerator) mapTypeInfo(mt *types.Map, tagFilter string) (typeScriptClientGeneratorField, error) {
	key := mt.Key()
	if nt, ok := key.(*types.Named); ok {
		key = nt.Underlying()
	}
	if b, ok := key.(*types.Basic); !ok || b.Kind() != types.String {
		return nil, fmt.Errorf("unsupported map key type: %s; only map[string]V is supported", mt.Key().String())
	}
	valueField, err := t.typeInfo(mt.Elem(), tagFilter)
	if err != nil {
		return nil, fmt.Errorf("failed to render map value type: %w", err)
	}
	return &typeScriptClientGeneratorMapField{value: valueField}, nil
}

func (t *typeScriptClientGenerator) typeNameByBasicLit(tt *types.Basic) (string, error) {
	switch tt.Kind() {
	case types.String, types.Int64, types.Uint64:
		return "string", nil
	case types.Int, types.Int8, types.Int16, types.Int32,
		types.Uint, types.Uint8, types.Uint16, types.Uint32,
		types.Float32, types.Float64, types.Complex64, types.Complex128:
		return "number", nil
	case types.Bool:
		return "boolean", nil

	}
	return "", fmt.Errorf("unsupported basic type: %s", tt.String())
}

// typeScriptClientGeneratorTemplateData is the top-level data passed to
// typescriptclient.tmpl. Routes drives the per-endpoint blocks (its methods —
// BuiltPaths, Methods — are reused from the previous template data). ErrorBody
// is non-nil only when WithErrorBody is registered on the router.
type typeScriptClientGeneratorTemplateData struct {
	Routes    typeScriptClientGeneratorTemplateArgs
	ErrorBody *typeScriptClientGeneratorErrorBody
}

type typeScriptClientGeneratorErrorBody struct {
	Field          typeScriptClientGeneratorField
	RequiredFields []typeScriptClientGeneratorErrorBodyField
}

func (e *typeScriptClientGeneratorErrorBody) RenderField(prefix string) string {
	return e.Field.RenderResponse(prefix)
}

type typeScriptClientGeneratorErrorBodyField struct {
	// Predicate is a complete TypeScript expression (operating on a
	// `Record<string, unknown>` named `e`) used to discriminate the error
	// response shape at runtime.
	Predicate string
}

type typeScriptClientGeneratorTemplateArgs []*typeScriptClientGeneratorTemplateArgsMethodPath

func (t typeScriptClientGeneratorTemplateArgs) BuiltPaths() []string {
	ss := make([]string, 0, len(t))
	smap := make(map[string]struct{})
	for _, mp := range t {
		s := mp.Builder()
		if s == "" {
			continue
		}
		if _, ok := smap[s]; ok {
			continue
		}
		smap[s] = struct{}{}
		ss = append(ss, s)
	}
	return ss
}

type typeScriptClientGeneratorTempalteArgsMethod string

func (t typeScriptClientGeneratorTempalteArgsMethod) Lower() string {
	return strings.ToLower(string(t))
}

func (t typeScriptClientGeneratorTempalteArgsMethod) LowerVar() string {
	if t == "DELETE" {
		return "_delete"
	}
	return t.Lower()
}

func (t typeScriptClientGeneratorTempalteArgsMethod) LowerReturn() string {
	if t == "DELETE" {
		return "delete: _delete"
	}
	return t.Lower()
}

func (t typeScriptClientGeneratorTempalteArgsMethod) Upper() string {
	return strings.ToUpper(string(t))
}

func (t typeScriptClientGeneratorTemplateArgs) Methods() []typeScriptClientGeneratorTempalteArgsMethod {
	methods := make([]typeScriptClientGeneratorTempalteArgsMethod, 0, len(t))
	mmap := make(map[typeScriptClientGeneratorTempalteArgsMethod]struct{})
	for _, mp := range t {
		m := typeScriptClientGeneratorTempalteArgsMethod(mp.Method)
		if _, ok := mmap[m]; ok {
			continue
		}
		mmap[m] = struct{}{}
		methods = append(methods, m)
	}
	return methods
}

type typeScriptClientGeneratorTemplateArgsMethodPath struct {
	Method   string
	Path     string
	Query    typeScriptClientGeneratorField
	Request  typeScriptClientGeneratorField
	Response typeScriptClientGeneratorField
}

func (t *typeScriptClientGeneratorTemplateArgsMethodPath) MethodPath() string {
	return fmt.Sprintf("%s %s", t.Method, t.Path)
}

func (t *typeScriptClientGeneratorTemplateArgsMethodPath) Builder() string {
	pathFragments := strings.Split(string(t.Path), "/")
	args := make([]string, 0, len(pathFragments))
	for _, fragment := range pathFragments {
		if !strings.HasPrefix(fragment, "{") || !strings.HasSuffix(fragment, "}") {
			continue
		}
		trimedFragment := strings.TrimPrefix(fragment, "{")
		trimedFragment = strings.TrimSuffix(trimedFragment, "}")
		hasRegexp := false
		argName := strings.Map(func(r rune) rune {
			if r == ':' || hasRegexp {
				hasRegexp = true
				return -1
			}
			return r
		}, trimedFragment)
		args = append(args, argName)
	}
	if len(args) == 0 {
		return ""
	}
	var argType strings.Builder
	for i, arg := range args {
		if i > 0 {
			argType.WriteString(", ")
		}
		argType.WriteString(fmt.Sprintf("%s: string", arg))
	}
	var builder strings.Builder
	for _, fragment := range pathFragments[1:] {
		builder.WriteString("/")
		if !strings.HasPrefix(fragment, "{") || !strings.HasSuffix(fragment, "}") {
			builder.WriteString(fragment)
			continue
		}
		var argName string
		argName, args = args[0], args[1:]
		builder.WriteString(fmt.Sprintf("${args.%s}", argName))
	}

	return fmt.Sprintf(`  "%s": (args: {%s}) => `+"`%s`", string(t.Path), argType.String(), builder.String())
}

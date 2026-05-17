package genclient

import (
	"bytes"
	"embed"
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

var jsonStringMarshalerWhitelist = map[string]struct{}{
	"time.Time": {},
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
	if len(result.RoutePaths) == 0 {
		return &bytes.Buffer{}, nil
	}

	gen, err := newTypeScriptClientGenerator()
	if err != nil {
		return nil, fmt.Errorf("failed to create TypeScript client generator: %w", err)
	}
	if err := gen.generate(result.RoutePaths, result.ErrorBody); err != nil {
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
			mp.Query = of
		}

		// json of request
		if of, err := t.typeInfo(h.Req(), "json"); err != nil {
			return fmt.Errorf("failed to generate request type of route %s %s: %w", r.Method(), r.Path(), err)
		} else {
			mp.Request = of
		}

		// json of response
		if of, err := t.typeInfo(h.Res(), "json"); err != nil {
			return fmt.Errorf("failed to generate response type of route %s %s: %w", r.Method(), r.Path(), err)
		} else {
			mp.Response = of
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
// to the TypeScript template: a rendered object field and the list of
// primitive json fields that should drive the isErrorResponse predicate.
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
		if gf.isOption || gf.isSlice {
			continue
		}
		lit, ok := gf.typedef.(typeScriptClientGeneratorLiteralType)
		if !ok {
			continue
		}
		kind := string(lit)
		if kind != "string" && kind != "number" && kind != "boolean" {
			continue
		}
		required = append(required, typeScriptClientGeneratorErrorBodyField{
			Name: gf.name,
			Kind: kind,
		})
	}
	return &typeScriptClientGeneratorErrorBody{
		Field:          obj,
		RequiredFields: required,
	}, nil
}

type typeScriptClientGeneratorField interface {
	RenderRequest(prefix string) string
	RenderResponse(prefix string) string
}

type typeScriptClientGeneratorObjectField struct {
	fields []typeScriptClientGeneratorField
}

func (t *typeScriptClientGeneratorObjectField) RenderRequest(prefix string) string {
	var ret strings.Builder
	ret.WriteString("{\n")
	for _, field := range t.fields {
		ret.WriteString(field.RenderRequest(prefix+"  ") + "\n")
	}
	ret.WriteString(prefix + "}")
	return ret.String()
}

func (t *typeScriptClientGeneratorObjectField) RenderResponse(prefix string) string {
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

func (t *typeScriptClientGenerator) typeInfo(tt types.Type, tagFilter string) (typeScriptClientGeneratorField, error) {
	if tp, ok := tt.(*types.Pointer); ok {
		tt = tp.Elem()
	}
	if field, ok, err := t.inertiaPageTypeInfo(tt, tagFilter); ok || err != nil {
		return field, err
	}
	var ts *types.Struct
	switch tt := tt.(type) {
	case *types.Struct:
		if tt.NumFields() == 0 {
			return &typeScriptClientGeneratorVoidField{}, nil
		}
		ts = tt
	case *types.Named:
		tu := tt.Underlying()
		return t.typeInfo(tu, tagFilter)
	default:
		return nil, fmt.Errorf("unsupported type: %s", tt.String())
	}
	fields, err := t.toFields(ts, tagFilter)
	if err != nil {
		return nil, fmt.Errorf("failed to convert fields: %w", err)
	}
	if len(fields) == 0 {
		return &typeScriptClientGeneratorVoidField{}, nil
	}

	return &typeScriptClientGeneratorObjectField{
		fields: fields,
	}, nil
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

		ft := f.Type()

		if nt, ok := ft.(*types.Named); ok {
			if _, ok := jsonStringMarshalerWhitelist[nt.String()]; ok {
				fields = append(fields, &typeScriptClientGeneratorGenericField{
					name:       fieldName,
					typedef:    typeScriptClientGeneratorLiteralType("string"),
					isSlice:    false,
					isRequired: required,
					isOption:   option,
				})
				continue
			}
			ft = nt.Underlying()
		}
		if pt, ok := ft.(*types.Pointer); ok {
			option = true
			ft = pt.Elem()
		}
		if nt, ok := ft.(*types.Named); ok {
			ft = nt.Underlying()
		}

		isSlice := false
		if st, ok := ft.(*types.Slice); ok {
			ft = st.Elem()
			isSlice = true
		}
		if pt, ok := ft.(*types.Pointer); ok {
			ft = pt.Elem()
		}
		if nt, ok := ft.(*types.Named); ok {
			ft = nt.Underlying()
		}

		if st, ok := ft.(*types.Struct); ok {
			cfs, err := t.toFields(st, filterTag)
			if err != nil {
				return nil, fmt.Errorf("failed to convert fields: %w", err)
			}
			if f.Embedded() {
				fields = append(fields, cfs...)
				continue
			}
			fields = append(fields, &typeScriptClientGeneratorGenericField{
				name:       fieldName,
				typedef:    &typeScriptClientGeneratorObjectField{fields: cfs},
				isSlice:    isSlice,
				isRequired: required,
				isOption:   option,
			})
			continue
		}

		if bt, ok := ft.(*types.Basic); ok {
			typename, err := t.typeNameByBasicLit(bt)
			if err != nil {
				return nil, fmt.Errorf("failed to convert basic type: %w", err)
			}
			fields = append(fields, &typeScriptClientGeneratorGenericField{
				name:       fieldName,
				typedef:    typeScriptClientGeneratorLiteralType(typename),
				isSlice:    isSlice,
				isRequired: required,
				isOption:   option,
			})
		} else {
			return nil, fmt.Errorf("unsupported field type: %s type=%T", ft.String(), ft)
		}

	}
	return fields, nil
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
	Name string
	Kind string
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

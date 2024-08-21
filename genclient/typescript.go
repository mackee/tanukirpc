package genclient

import (
	"bytes"
	"embed"
	"fmt"
	"go/types"
	"io"
	"os"
	"reflect"
	"strings"
	"text/template"

	"golang.org/x/tools/go/analysis"
)

//go:embed typescriptclient.tmpl
var typeScriptClientTemplate embed.FS

var TypeScriptClientGenerator = &analysis.Analyzer{
	Name: "gentypescript",
	Doc:  "generate TypeScript client code",
	Run:  generateTypeScriptClient,
	Requires: []*analysis.Analyzer{
		Analyzer,
	},
	ResultType: reflect.TypeOf((*bytes.Buffer)(nil)),
}

var typeScriptClientOutPath string

func init() {
	TypeScriptClientGenerator.Flags.StringVar(&typeScriptClientOutPath, "out", "", "output file path")
}

func generateTypeScriptClient(pass *analysis.Pass) (any, error) {
	result := pass.ResultOf[Analyzer].(*AnalyzerResult)
	gen, err := newTypeScriptClientGenerator()
	if err != nil {
		return nil, fmt.Errorf("failed to create TypeScript client generator: %w", err)
	}
	if err := gen.generate(result.RoutePaths); err != nil {
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

func (t *typeScriptClientGenerator) generate(routes []RoutePath) error {
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
	if err := t.tmpl.Execute(t.rw, templateArgs); err != nil {
		return fmt.Errorf("failed to execute template: %w", err)
	}
	return nil
}

type typeScriptClientGeneratorField interface {
	RenderRequest(prefix string) string
	RenderResponse(prefix string) string
}

type typeScriptClientGeneratorObjectField struct {
	fields []typeScriptClientGeneratorField
}

func (t *typeScriptClientGeneratorObjectField) RenderRequest(prefix string) string {
	ret := "{\n"
	for _, field := range t.fields {
		ret += field.RenderRequest(prefix+"  ") + "\n"
	}
	ret += prefix + "};"
	return ret
}

func (t *typeScriptClientGeneratorObjectField) RenderResponse(prefix string) string {
	ret := "{\n"
	for _, field := range t.fields {
		ret += field.RenderResponse(prefix+"  ") + "\n"
	}
	ret += prefix + "};"
	return ret
}

type typeScriptClientGeneratorLiteralField struct {
	name       string
	typename   string
	isSlice    bool
	isRequired bool
	isOption   bool
}

func (t *typeScriptClientGeneratorLiteralField) sliceSuffix() string {
	if t.isSlice {
		return "[]"
	}
	return ""
}

func (t *typeScriptClientGeneratorLiteralField) isRequiredOpRequest() string {
	if t.isRequired {
		return ""
	}
	return "?"
}

func (t *typeScriptClientGeneratorLiteralField) isRequiredOpResponse() string {
	if t.isOption {
		return "?"
	}
	return ""
}

func (t *typeScriptClientGeneratorLiteralField) RenderRequest(prefix string) string {
	return fmt.Sprintf("%s%s%s: %s%s", prefix, t.name, t.isRequiredOpRequest(), t.typename, t.sliceSuffix())
}

func (t *typeScriptClientGeneratorLiteralField) RenderResponse(prefix string) string {
	return fmt.Sprintf("%s%s%s: %s%s", prefix, t.name, t.isRequiredOpResponse(), t.typename, t.sliceSuffix())
}

type typeScriptClientGeneratorVoidField struct{}

func (t *typeScriptClientGeneratorVoidField) RenderRequest(prefix string) string {
	return "undefined;"
}

func (t *typeScriptClientGeneratorVoidField) RenderResponse(prefix string) string {
	return "undefined;"
}

func (t *typeScriptClientGenerator) typeInfo(tt types.Type, tagFilter string) (typeScriptClientGeneratorField, error) {
	if tp, ok := tt.(*types.Pointer); ok {
		tt = tp.Elem()
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

		ft := f.Type()
		if pt, ok := ft.(*types.Pointer); ok {
			option = true
			ft = pt.Elem()
		}

		isSlice := false
		if st, ok := ft.(*types.Slice); ok {
			ft = st.Elem()
			isSlice = true
		}
		if bt, ok := ft.(*types.Basic); ok {
			typename, err := t.typeNameByBasicLit(bt)
			if err != nil {
				return nil, fmt.Errorf("failed to convert basic type: %w", err)
			}
			fields = append(fields, &typeScriptClientGeneratorLiteralField{
				name:       fieldName,
				typename:   typename,
				isSlice:    isSlice,
				isRequired: required,
				isOption:   option,
			})
		} else {
			return nil, fmt.Errorf("unsupported field type: %s", ft.String())
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

	}
	return "", fmt.Errorf("unsupported basic type: %s", tt.String())
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
		ss = append(ss, s)
	}
	return ss
}

type typeScriptClientGeneratorTempalteArgsMethod string

func (t typeScriptClientGeneratorTempalteArgsMethod) Lower() string {
	return strings.ToLower(string(t))
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
	argType := ""
	for i, arg := range args {
		if i > 0 {
			argType += ", "
		}
		argType += fmt.Sprintf("%s: string", arg)
	}
	builder := ""
	for _, fragment := range pathFragments[1:] {
		builder += "/"
		if !strings.HasPrefix(fragment, "{") || !strings.HasSuffix(fragment, "}") {
			builder += fragment
			continue
		}
		var argName string
		argName, args = args[0], args[1:]
		builder += fmt.Sprintf("${args.%s}", argName)
	}

	return fmt.Sprintf(`  "%s": (args: {%s}) => `+"`%s`", string(t.Path), argType, builder)
}

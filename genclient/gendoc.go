package genclient

import (
	"go/types"
	"net/http"
	"path"
	"reflect"
	"strconv"

	"github.com/gostaticanalysis/analysisutil"
	"github.com/mackee/tanukirpc"
	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/buildssa"
	"golang.org/x/tools/go/analysis/passes/inspect"
	"golang.org/x/tools/go/ssa"
)

func AnalyzeTarget[Reg any](router *tanukirpc.Router[Reg]) {
	// for static analysis
}

var Analyzer = &analysis.Analyzer{
	Name: "tanukirpc",
	Doc:  "This is a static analysis tool for tanukirpc.",
	Run:  run,
	Requires: []*analysis.Analyzer{
		inspect.Analyzer,
		buildssa.Analyzer,
	},
	ResultType: reflect.TypeOf(&AnalyzerResult{}),
}

type AnalyzerResult struct {
	RoutePaths []RoutePath
}

var routerMethodNames = map[string]string{
	"Get":     http.MethodGet,
	"Post":    http.MethodPost,
	"Put":     http.MethodPut,
	"Delete":  http.MethodDelete,
	"Patch":   http.MethodPatch,
	"Head":    http.MethodHead,
	"Options": http.MethodOptions,
	"Trace":   http.MethodTrace,
	"Connect": http.MethodConnect,
}

func run(pass *analysis.Pass) (any, error) {
	ap := newTanukiTypeInfo(pass)
	if ap == nil {
		return &AnalyzerResult{}, nil
	}
	analyzeTargetObj := analysisutil.LookupFromImports(
		pass.Pkg.Imports(),
		"github.com/mackee/tanukirpc/genclient",
		"AnalyzeTarget",
	)

	ssaresult := pass.ResultOf[buildssa.Analyzer].(*buildssa.SSA)
	rps := make([]RoutePath, 0)
	for _, f := range ssaresult.SrcFuncs {
		for _, b := range f.Blocks {
			routerArgs := make([]ssa.Value, 0)
			for _, instr := range b.Instrs {
				call, ok := instr.(*ssa.Call)
				if !ok {
					continue
				}
				sf, ok := call.Call.Value.(*ssa.Function)
				if !ok {
					continue
				}
				if sf.Object() != analyzeTargetObj {
					continue
				}
				args := call.Call.Args
				if len(args) < 1 {
					pass.Reportf(call.Pos(), "invalid number of arguments")
					continue
				}
				routerArgs = append(routerArgs, args[0])
			}
			if len(routerArgs) == 0 {
				continue
			}
			for _, arg := range routerArgs {
				is := ap.analyzeRouterValue(pass, arg)
				for _, rp := range is.listRoute() {
					rps = append(rps, rp)
				}
			}
		}
	}

	return &AnalyzerResult{
		RoutePaths: rps,
	}, nil
}

func (g *tanukiTypeInfo) analyzeRouterValue(pass *analysis.Pass, v ssa.Value) *instrs {
	routerInstrs := make([]ssa.Instruction, 0)
	if call, ok := v.(*ssa.Call); ok {
		routerInstrs = append(routerInstrs, call)
	}

	referrers := v.Referrers()
	if referrers != nil {
		routerInstrs = append(routerInstrs, *referrers...)
	}
	is := &instrs{
		agg:    g,
		instrs: routerInstrs,
	}
	is.analyze(pass)

	return is
}

type tanukiTypeInfo struct {
	routerObj               types.Object
	newHandlerObj           types.Object
	routerMethods           map[*types.Func]string
	routeMethod             *types.Func
	routeWithTransformerObj types.Object
}

func newTanukiTypeInfo(pass *analysis.Pass) *tanukiTypeInfo {
	routerObj := analysisutil.LookupFromImports(pass.Pkg.Imports(), "github.com/mackee/tanukirpc", "Router")
	if routerObj == nil {
		return nil
	}
	routerMethods := make(map[*types.Func]string, len(routerMethodNames))
	for mn, method := range routerMethodNames {
		rm := analysisutil.MethodOf(routerObj.Type(), mn)
		routerMethods[rm] = method
	}
	routeMethod := analysisutil.MethodOf(routerObj.Type(), "Route")
	newHandlerObj := analysisutil.LookupFromImports(
		pass.Pkg.Imports(),
		"github.com/mackee/tanukirpc",
		"NewHandler",
	)
	routeWithTransformerObj := analysisutil.LookupFromImports(
		pass.Pkg.Imports(),
		"github.com/mackee/tanukirpc",
		"RouteWithTransformer",
	)

	return &tanukiTypeInfo{
		routerObj:               routerObj,
		newHandlerObj:           newHandlerObj,
		routerMethods:           routerMethods,
		routeMethod:             routeMethod,
		routeWithTransformerObj: routeWithTransformerObj,
	}
}

func (g *tanukiTypeInfo) isRouterType(t types.Type) bool {
	if pt, ok := t.(*types.Pointer); ok {
		t = pt.Elem()
	}
	if nt, ok := t.(*types.Named); ok {
		t = nt.Origin()
	}
	return types.Identical(t, g.routerObj.Type())
}

type analyzedPath interface {
	joinPath(p string) string
	listRoute() []*routePath
}

type instrs struct {
	agg      *tanukiTypeInfo
	parent   analyzedPath
	instrs   []ssa.Instruction
	children []analyzedPath
}

func (i *instrs) joinPath(p string) string {
	if i.parent == nil {
		return p
	}
	return i.parent.joinPath(p)
}

func (i *instrs) listRoute() []*routePath {
	rp := make([]*routePath, 0)
	for _, c := range i.children {
		rp = append(rp, c.listRoute()...)
	}

	return rp
}

func (i *instrs) analyze(pass *analysis.Pass) {
	for _, instr := range i.instrs {
		switch instr := instr.(type) {
		case *ssa.Call:
			if rnp := i.tryRouteWithTransformer(pass, instr); rnp != nil {
				i.children = append(i.children, rnp)
				continue
			}
			if rnp := i.tryRoute(pass, instr); rnp != nil {
				i.children = append(i.children, rnp)
				continue
			}
			if rp := i.tryPathMethod(pass, instr); rp != nil {
				i.children = append(i.children, rp)
				continue
			}
			if callee := instr.Call.StaticCallee(); callee != nil {
				if extract := i.extractCallee(callee); extract != nil {
					extract.analyze(pass)
					i.children = append(i.children, extract)
					continue
				}
			}
		}
	}
}

func (i *instrs) extractCallee(callee *ssa.Function) *instrs {
	is := make([]ssa.Instruction, 0)
	returns := analysisutil.Returns(callee)
	for _, ret := range returns {
		for _, result := range ret.Results {
			if !i.agg.isRouterType(result.Type()) {
				continue
			}
			referrers := result.Referrers()
			if referrers == nil {
				continue
			}
			is = append(is, *referrers...)
		}
	}
	return &instrs{
		agg:    i.agg,
		parent: i,
		instrs: is,
	}
}

type routeNestedPath struct {
	parent   analyzedPath
	path     string
	children *instrs
}

func (i *instrs) tryRoute(pass *analysis.Pass, instr ssa.Instruction) *routeNestedPath {
	call, ok := instr.(*ssa.Call)
	if !ok {
		return nil
	}
	callee := call.Call.StaticCallee()
	if callee == nil {
		return nil
	}
	named, ok := callee.Object().(*types.Func)
	if !ok {
		return nil
	}
	if named.Origin() != i.agg.routeMethod {
		return nil
	}
	args := call.Call.Args
	if len(args) != 3 {
		pass.Reportf(call.Pos(), "invalid number of arguments")
		return nil
	}
	pathArg := args[1]
	c, ok := pathArg.(*ssa.Const)
	if !ok {
		pass.Reportf(pathArg.Pos(), "invalid path argument. must be string literal.")
		return nil
	}
	if c.Value == nil {
		pass.Reportf(pathArg.Pos(), "invalid path argument. must be string literal.")
		return nil
	}

	handlerArg := args[2]
	children := i.routeHandlerFuncToInstrs(pass, handlerArg)
	if children == nil {
		return nil
	}

	np := &routeNestedPath{
		parent:   i,
		path:     c.Value.ExactString(),
		children: children,
	}
	children.parent = np
	children.analyze(pass)

	return np
}

func (i *instrs) routeHandlerFuncToInstrs(pass *analysis.Pass, v ssa.Value) *instrs {
	if closure, ok := v.(*ssa.MakeClosure); ok {
		v = closure.Fn
	}

	childFunc, ok := v.(*ssa.Function)
	if !ok {
		pass.Reportf(v.Pos(), "invalid handler argument. must be function literal.")
		return nil
	}
	cps := childFunc.Params
	if len(cps) != 1 {
		pass.Reportf(v.Pos(), "invalid handler argument. must be function literal.")
		return nil
	}
	routerParam := cps[0]
	is := make([]ssa.Instruction, 0)
	if referrers := routerParam.Referrers(); referrers != nil {
		is = append(is, *referrers...)
	}

	return &instrs{
		agg:    i.agg,
		instrs: is,
	}
}

func (i *instrs) tryRouteWithTransformer(pass *analysis.Pass, instr ssa.Instruction) *routeNestedPath {
	call, ok := instr.(*ssa.Call)
	if !ok {
		return nil
	}
	callee := call.Call.StaticCallee()
	if callee == nil {
		return nil
	}
	named, ok := callee.Object().(*types.Func)
	if !ok {
		return nil
	}
	if named.Origin() != i.agg.routeWithTransformerObj {
		return nil
	}
	args := call.Call.Args
	if len(args) != 4 {
		pass.Reportf(call.Pos(), "invalid number of arguments")
		return nil
	}
	pathArg := args[2]
	c, ok := pathArg.(*ssa.Const)
	if !ok {
		pass.Reportf(pathArg.Pos(), "invalid path argument. must be string literal.")
		return nil
	}
	if c.Value == nil {
		pass.Reportf(pathArg.Pos(), "invalid path argument. must be string literal.")
		return nil
	}

	handlerArg := args[3]
	children := i.routeHandlerFuncToInstrs(pass, handlerArg)
	if children == nil {
		return nil
	}

	np := &routeNestedPath{
		parent:   i,
		path:     c.Value.ExactString(),
		children: children,
	}
	children.parent = np
	children.analyze(pass)

	return np
}

func (r *routeNestedPath) joinPath(p string) string {
	unquoted, _ := strconv.Unquote(r.path)
	return r.parent.joinPath(path.Join(unquoted, p))
}

func (r *routeNestedPath) listRoute() []*routePath {
	return r.children.listRoute()
}

type routePath struct {
	parent  analyzedPath
	path    string
	method  string
	handler *handlerType
}

type RoutePath interface {
	Path() string
	Method() string
	Handler() HandlerType
}

func (r *routePath) Path() string {
	return r.joinPath("")
}

func (r *routePath) Method() string {
	return r.method
}

func (r *routePath) Handler() HandlerType {
	return r.handler
}

type handlerType struct {
	req types.Type
	res types.Type
	reg types.Type
}

type HandlerType interface {
	Req() types.Type
	Res() types.Type
	Reg() types.Type
}

func (h *handlerType) Req() types.Type {
	return h.req
}

func (h *handlerType) Res() types.Type {
	return h.res
}

func (h *handlerType) Reg() types.Type {
	return h.reg
}

func (i *instrs) tryPathMethod(pass *analysis.Pass, instr ssa.Instruction) *routePath {
	call, ok := instr.(*ssa.Call)
	if !ok {
		return nil
	}
	callee := call.Call.StaticCallee()
	if callee == nil {
		return nil
	}
	named, ok := callee.Object().(*types.Func)
	if !ok {
		return nil
	}
	orig := named.Origin()
	httpMethod, ok := i.agg.routerMethods[orig]
	if !ok {
		return nil
	}

	args := call.Call.Args
	if len(args) != 3 {
		pass.Reportf(call.Pos(), "invalid number of arguments")
		return nil
	}
	pathArg := args[1]
	c, ok := pathArg.(*ssa.Const)
	if !ok {
		pass.Reportf(pathArg.Pos(), "invalid path argument. must be string literal.")
		return nil
	}
	if c.Value == nil {
		pass.Reportf(pathArg.Pos(), "invalid path argument. must be string literal.")
		return nil
	}
	pathStr := c.Value.ExactString()

	handlerArg := args[2]
	ht := i.handlerType(pass, handlerArg)
	if ht == nil {
		return nil
	}

	return &routePath{
		parent:  i,
		path:    pathStr,
		method:  httpMethod,
		handler: ht,
	}
}

func (i *instrs) handlerType(pass *analysis.Pass, v ssa.Value) *handlerType {
	call, ok := v.(*ssa.Call)
	if !ok {
		return nil
	}
	callee := call.Call.StaticCallee()
	if callee == nil {
		return nil
	}
	fn, ok := callee.Object().(*types.Func)
	if !ok {
		return nil
	}
	if fn != i.agg.newHandlerObj {
		pass.Reportf(call.Pos(), "invalid handler argument. must be NewHandler function call.")
		return nil
	}

	instance := call.Call.Signature()
	tp := instance.Params().At(0)
	tpn, ok := tp.Type().(*types.Named)
	if !ok {
		pass.Reportf(call.Pos(), "invalid handler argument. must be NewHandler function call.")
		return nil
	}
	tps := tpn.TypeArgs()
	req := tps.At(0)
	res := tps.At(1)
	reg := tps.At(2)
	return &handlerType{
		req: req,
		res: res,
		reg: reg,
	}
}

func (r *routePath) joinPath(p string) string {
	unquoted, _ := strconv.Unquote(r.path)
	return r.parent.joinPath(path.Join(unquoted, p))
}

func (r *routePath) listRoute() []*routePath {
	return []*routePath{r}
}

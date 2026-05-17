package genclient

import (
	"fmt"
	"go/token"
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
	ResultType: reflect.TypeFor[*AnalyzerResult](),
}

type AnalyzerResult struct {
	RoutePaths []RoutePath
	// ErrorBody is the application-defined error body type registered via
	// tanukirpc.WithErrorBody. nil when the analyzed router does not use it,
	// in which case generators fall back to the default error shape.
	ErrorBody types.Type
	// UnresolvedOptions lists source positions of RouterOption arguments that
	// the analyzer could not classify statically (e.g. options loaded from
	// package-level variables, *ssa.Phi, or external callees). Generators
	// re-emit these as diagnostics so users notice when WithErrorBody might
	// silently fall back to the default error shape.
	UnresolvedOptions []token.Pos
	// MultipleAnalyzeTargetPositions lists the positions of every
	// genclient.AnalyzeTarget call when the analyzed package has more than one.
	// A single generated client.ts can only represent one router's routes and
	// error body, so the generator rejects this configuration by emitting an
	// error diagnostic at each call site instead of guessing which router to
	// honor.
	MultipleAnalyzeTargetPositions []token.Pos
	// AnalyzeTargetCallPos is the position of the single accepted
	// genclient.AnalyzeTarget call. Generators use it as a fallback diagnostic
	// site when an underlying SSA value has no source position (e.g. an
	// anonymous-struct error body has no named-type Pos to attach a warning
	// to).
	AnalyzeTargetCallPos token.Pos
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

	// First collect every AnalyzeTarget call so we can reject the
	// multiple-routers-per-package case before doing any work. Generating one
	// client.ts with two different error bodies would silently mismatch one of
	// the routers at runtime, so we treat this as a hard error.
	type analyzeTargetCall struct {
		call    *ssa.Call
		routers []ssa.Value
	}
	var atCalls []analyzeTargetCall
	for _, f := range ssaresult.SrcFuncs {
		for _, b := range f.Blocks {
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
				atCalls = append(atCalls, analyzeTargetCall{call: call, routers: args[:1]})
			}
		}
	}
	if len(atCalls) > 1 {
		positions := make([]token.Pos, 0, len(atCalls))
		for _, c := range atCalls {
			positions = append(positions, c.call.Pos())
		}
		return &AnalyzerResult{MultipleAnalyzeTargetPositions: positions}, nil
	}

	rps := make([]RoutePath, 0)
	var errorBody types.Type
	var unresolved []token.Pos
	var analyzeTargetPos token.Pos
	if len(atCalls) == 1 {
		analyzeTargetPos = atCalls[0].call.Pos()
	}
	for _, atc := range atCalls {
		for _, arg := range atc.routers {
			is := ap.analyzeRouterValue(pass, arg)
			for _, rp := range is.listRoute() {
				rps = append(rps, rp)
			}
			eb, unresPositions := ap.extractErrorBody(arg, atc.call.Pos())
			if errorBody == nil && eb != nil {
				errorBody = eb
			}
			unresolved = append(unresolved, unresPositions...)
		}
	}

	return &AnalyzerResult{
		RoutePaths:           rps,
		ErrorBody:            errorBody,
		UnresolvedOptions:    unresolved,
		AnalyzeTargetCallPos: analyzeTargetPos,
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
	newRouterObj            types.Object
	withErrorBodyObj        types.Object
	withErrorHookerObj      types.Object
	// otherOptionObjs holds the RouterOption builder objects that do not affect
	// error rendering (WithCodec, WithLogger, ...). Used by classifyOption to
	// silently accept these instead of flagging them as unknown.
	otherOptionObjs map[types.Object]bool
	// routerPreservingMethods is the set of *Router[Reg] methods that return a
	// router which inherits the parent's errorHooker (currently With and
	// Route). When AnalyzeTarget receives a router produced by one of these,
	// the analyzer follows the call's receiver back to the originating router.
	routerPreservingMethods map[*types.Func]bool
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
	newRouterObj := analysisutil.LookupFromImports(
		pass.Pkg.Imports(),
		"github.com/mackee/tanukirpc",
		"NewRouter",
	)
	withErrorBodyObj := analysisutil.LookupFromImports(
		pass.Pkg.Imports(),
		"github.com/mackee/tanukirpc",
		"WithErrorBody",
	)
	withErrorHookerObj := analysisutil.LookupFromImports(
		pass.Pkg.Imports(),
		"github.com/mackee/tanukirpc",
		"WithErrorHooker",
	)
	otherOptionObjs := map[types.Object]bool{}
	for _, name := range []string{
		"WithChiRouter",
		"WithCodec",
		"WithContextFactory",
		"WithLogger",
		"WithAccessLogger",
		"WithDefaultMiddleware",
	} {
		if obj := analysisutil.LookupFromImports(pass.Pkg.Imports(), "github.com/mackee/tanukirpc", name); obj != nil {
			otherOptionObjs[obj] = true
		}
	}
	routerPreservingMethods := map[*types.Func]bool{}
	for _, name := range []string{"With", "Route"} {
		if m := analysisutil.MethodOf(routerObj.Type(), name); m != nil {
			routerPreservingMethods[m] = true
		}
	}

	return &tanukiTypeInfo{
		routerObj:               routerObj,
		newHandlerObj:           newHandlerObj,
		routerMethods:           routerMethods,
		routeMethod:             routeMethod,
		routeWithTransformerObj: routeWithTransformerObj,
		newRouterObj:            newRouterObj,
		withErrorBodyObj:        withErrorBodyObj,
		withErrorHookerObj:      withErrorHookerObj,
		otherOptionObjs:         otherOptionObjs,
		routerPreservingMethods: routerPreservingMethods,
	}
}

// extractErrorBody walks back from a router SSA value to the NewRouter call
// that produced it and classifies each RouterOption argument. Returns the
// last-wins WithErrorBody type (or nil) along with source positions of options
// that could not be statically resolved.
//
// When v is the result of a helper function that returns a *Router (the
// gendoctest pattern), the helper's return values are recursively analyzed so
// that WithErrorBody calls inside the factory are still picked up.
func (g *tanukiTypeInfo) extractErrorBody(v ssa.Value, fallbackPos token.Pos) (types.Type, []token.Pos) {
	return g.extractErrorBodyRec(v, fallbackPos, map[*ssa.Function]bool{})
}

func (g *tanukiTypeInfo) extractErrorBodyRec(v ssa.Value, fallbackPos token.Pos, visited map[*ssa.Function]bool) (types.Type, []token.Pos) {
	if g.withErrorBodyObj == nil || g.newRouterObj == nil {
		return nil, nil
	}
	// Same-package generic factories surface their return value through an
	// *ssa.ChangeType (Router[Reg] → Router[concrete]). Unwrap so we see the
	// underlying NewRouter call.
	v = unwrapValueConversions(v)
	call, ok := v.(*ssa.Call)
	if !ok {
		// Router-typed value the analyzer cannot trace back to a NewRouter
		// call (e.g. *ssa.Phi from if/else, *ssa.Extract from multi-return,
		// *ssa.UnOp deref of a local pointer, *ssa.TypeAssert, function
		// parameter). Warn at the AnalyzeTarget call site so users don't see
		// a silent fallback when their runtime router was built with
		// WithErrorBody.
		if g.isRouterType(v.Type()) {
			return nil, []token.Pos{fallbackPos}
		}
		return nil, nil
	}
	callee, ok := call.Call.Value.(*ssa.Function)
	if !ok {
		return nil, nil
	}
	if isSameOriginFunc(callee.Object(), g.newRouterObj) {
		// NewRouter[Reg](reg, opts ...RouterOption[Reg]): variadic opts arrive
		// as a single slice value (args[len-1]) in SSA. That slice may be the
		// synthetic varargs allocation the compiler emits for inline option
		// lists, or it may be an opaque slice the caller built and spread with
		// `...`. collectOptionElements unwraps both shapes; if it cannot, the
		// spread site is reported so callers don't see a silent fallback.
		args := call.Call.Args
		if len(args) < 2 {
			return nil, nil
		}
		variadic := args[len(args)-1]
		elems, resolved, divergent := g.collectOptionElements(variadic)
		eb, unres := g.errorBodyFromOpts(elems)
		if !resolved {
			unres = append(unres, variadic.Pos())
		}
		unres = append(unres, divergent...)
		return eb, unres
	}
	// Router-preserving methods (r.With(...), r.Route(...)) return a clone
	// that inherits the parent's errorHooker. Follow the receiver to discover
	// any WithErrorBody registered on the original router.
	if obj, ok := callee.Object().(*types.Func); ok && g.routerPreservingMethods[obj.Origin()] {
		if len(call.Call.Args) >= 1 {
			return g.extractErrorBodyRec(call.Call.Args[0], fallbackPos, visited)
		}
	}
	// Not a direct NewRouter call: recurse into the callee's return values that
	// yield a *Router. Branching factories (multiple returns with different
	// shapes) keep the first-wins error body to preserve the typed
	// ErrorResponse, but the helper call site is reported so the user knows
	// runtime may not match.
	if visited[callee] {
		return nil, nil
	}
	if len(callee.Blocks) == 0 {
		// External / synthetic function whose body is not part of the
		// analyzed package's SSA. We cannot see whether it registers a
		// WithErrorBody, so warn at the call site instead of silently
		// producing the default error shape.
		return nil, []token.Pos{call.Pos()}
	}
	visited[callee] = true
	var (
		allUnresolved  []token.Pos
		firstEB        types.Type
		distinctBodies = map[string]bool{}
	)
	for _, ret := range analysisutil.Returns(callee) {
		for _, result := range ret.Results {
			if !g.isRouterType(result.Type()) {
				continue
			}
			eb, unres := g.extractErrorBodyRec(result, fallbackPos, visited)
			allUnresolved = append(allUnresolved, unres...)
			distinctBodies[errorBodyKey(eb)] = true
			if firstEB == nil && eb != nil {
				firstEB = eb
			}
		}
	}
	if len(distinctBodies) > 1 {
		allUnresolved = append(allUnresolved, call.Pos())
	}
	return firstEB, allUnresolved
}

// errorBodyKey turns a types.Type into a stable key for distinct-result checks.
// nil and the empty string are both distinct sentinel values.
func errorBodyKey(t types.Type) string {
	if t == nil {
		return "<nil>"
	}
	return t.String()
}

// collectOptionElements turns the SSA value passed as the variadic argument of
// NewRouter into the individual RouterOption values. Recognized shapes:
//
//  1. The synthetic slice the compiler builds for inline option lists
//     (Alloc + IndexAddr + Store + *ssa.Slice). Handled by
//     collectVariadicElements.
//  2. A slice produced elsewhere and spread with `...`, e.g.
//     `tanukirpc.NewRouter(reg, buildOptions()...)`. In SSA the variadic
//     argument is the call result; we recurse into the callee's returns and
//     keep unwrapping until we hit a synthetic slice or run out of paths.
//  3. A literal nil slice (the call site simply passed no extra options).
//
// Returns:
//   - elems: the flattened option list (across all return paths if multiple).
//   - resolved: false when at least one shape could not be resolved (callers
//     should warn at the spread site).
//   - divergent: positions of helper calls whose return paths classify into
//     different error-body shapes. Callers re-emit these as diagnostics so the
//     user notices that a `buildOptions(flag)...` spread may hide a
//     flag-dependent error body.
func (g *tanukiTypeInfo) collectOptionElements(v ssa.Value) (elems []ssa.Value, resolved bool, divergent []token.Pos) {
	return g.collectOptionElementsRec(v, map[*ssa.Function]bool{})
}

func (g *tanukiTypeInfo) collectOptionElementsRec(v ssa.Value, visited map[*ssa.Function]bool) ([]ssa.Value, bool, []token.Pos) {
	// Strip generic-call result wrappers (*ssa.ChangeType / *ssa.Convert) so
	// the *ssa.Call check below also matches `buildOptions[Reg]()...` shapes
	// the same way unwrapValueConversions handles generic router factories.
	v = unwrapValueConversions(v)
	if elems, ok := collectVariadicElements(v); elems != nil {
		return elems, ok, nil
	}
	// `tanukirpc.NewRouter(reg)` lowers the missing variadic to a nil slice
	// constant; treat that as resolved-empty so we don't warn on the common
	// "no extra options" form.
	if c, ok := v.(*ssa.Const); ok && c.Value == nil {
		return nil, true, nil
	}
	call, ok := v.(*ssa.Call)
	if !ok {
		return nil, false, nil
	}
	callee, ok := call.Call.Value.(*ssa.Function)
	if !ok {
		return nil, false, nil
	}
	if visited[callee] || len(callee.Blocks) == 0 {
		return nil, false, nil
	}
	visited[callee] = true
	type branchResult struct {
		elems    []ssa.Value
		resolved bool
		warns    []token.Pos
	}
	var branches []branchResult
	for _, ret := range analysisutil.Returns(callee) {
		for _, result := range ret.Results {
			sub, res, warns := g.collectOptionElementsRec(result, visited)
			branches = append(branches, branchResult{elems: sub, resolved: res, warns: warns})
		}
	}
	var (
		all            []ssa.Value
		resolved       = true
		warnPositions  []token.Pos
		distinctBodies = map[string]bool{}
	)
	for _, b := range branches {
		all = append(all, b.elems...)
		if !b.resolved {
			resolved = false
		}
		warnPositions = append(warnPositions, b.warns...)
		if b.resolved {
			// Classify this branch in isolation. If branches disagree about the
			// error body shape, the user is taking different runtime paths and
			// the generated TS cannot reflect both — warn at the helper call.
			eb, _ := g.errorBodyFromOpts(b.elems)
			distinctBodies[errorBodyKey(eb)] = true
		}
	}
	if len(distinctBodies) > 1 {
		warnPositions = append(warnPositions, call.Pos())
	}
	return all, resolved, warnPositions
}

// errorBodyFromOpts walks the variadic option list in the order it would be
// applied by Router.apply: the last WithErrorBody wins, and any WithErrorHooker
// appearing after it overrides the body entirely (the analyzer treats that as
// "no custom ErrorResponse"). Options that cannot be statically classified
// have their source positions returned so the caller can surface a diagnostic
// and prevent silent generation/runtime mismatches.
func (g *tanukiTypeInfo) errorBodyFromOpts(opts []ssa.Value) (types.Type, []token.Pos) {
	var current types.Type
	var unresolved []token.Pos
	for _, opt := range opts {
		cls := g.classifyOption(opt)
		if cls.conflicted {
			unresolved = append(unresolved, opt.Pos())
		}
		switch cls.kind {
		case optErrorBody:
			current = cls.errorBody
		case optErrorHooker:
			current = nil
		case optUnresolved:
			unresolved = append(unresolved, opt.Pos())
		}
	}
	return current, unresolved
}

type optKind int

const (
	optIrrelevant optKind = iota // statically resolved, has no effect on the error body
	optErrorBody                 // WithErrorBody (directly or via a wrapper that returns one)
	optErrorHooker               // WithErrorHooker (directly or via a wrapper that returns one)
	optUnresolved                // could not be statically resolved; user should be warned
)

type optResult struct {
	kind      optKind
	errorBody types.Type // populated when kind == optErrorBody
	// conflicted is set when a wrapper had multiple return paths whose
	// classifications disagree (e.g. one branch returns WithErrorBody, another
	// returns WithErrorHooker). The first-wins kind is still reported, but
	// callers should warn so the user notices the mismatch.
	conflicted bool
}

// classifyOption decides how a single RouterOption value affects error
// rendering. The classifier recurses into wrapper functions that return a
// RouterOption, so patterns like `func appErrorBody() RouterOption { return
// WithErrorBody(...) }` are picked up. Values that aren't a direct *ssa.Call
// (e.g. *ssa.Phi, *ssa.UnOp from a global) or callees whose body is unavailable
// (other packages) classify as optUnresolved.
func (g *tanukiTypeInfo) classifyOption(v ssa.Value) optResult {
	return g.classifyOptionRec(v, map[*ssa.Function]bool{})
}

func (g *tanukiTypeInfo) classifyOptionRec(v ssa.Value, visited map[*ssa.Function]bool) optResult {
	v = unwrapValueConversions(v)
	call, ok := v.(*ssa.Call)
	if !ok {
		return optResult{kind: optUnresolved}
	}
	callee, ok := call.Call.Value.(*ssa.Function)
	if !ok {
		return optResult{kind: optUnresolved}
	}
	obj := callee.Object()
	if isSameOriginFunc(obj, g.withErrorBodyObj) {
		if eb := g.errorBodyTypeArg(call); eb != nil {
			return optResult{kind: optErrorBody, errorBody: eb}
		}
		return optResult{kind: optUnresolved}
	}
	if isSameOriginFunc(obj, g.withErrorHookerObj) {
		return optResult{kind: optErrorHooker}
	}
	if obj != nil {
		if f, ok := obj.(*types.Func); ok {
			if g.otherOptionObjs[f] || g.otherOptionObjs[f.Origin()] {
				return optResult{kind: optIrrelevant}
			}
		}
	}
	// Unknown callee: recurse into its returns. External / generic callees with
	// no SSA body classify as unresolved so the user gets a warning.
	if visited[callee] || len(callee.Blocks) == 0 {
		return optResult{kind: optUnresolved}
	}
	visited[callee] = true
	// Aggregate over all return paths. Preference (when results disagree):
	// ErrorBody > ErrorHooker > Unresolved > Irrelevant — keeps the optimistic
	// typed ErrorResponse for the user. If multiple distinct classifications
	// surface across branches the result is marked conflicted so callers can
	// emit a warning.
	var (
		aggregate    = optResult{kind: optIrrelevant}
		seen         = map[string]bool{}
		distinctKeys int
	)
	bumpDistinct := func(key string) {
		if !seen[key] {
			seen[key] = true
			distinctKeys++
		}
	}
	for _, ret := range analysisutil.Returns(callee) {
		for _, result := range ret.Results {
			sub := g.classifyOptionRec(result, visited)
			bumpDistinct(classifyKey(sub))
			if sub.conflicted {
				aggregate.conflicted = true
			}
			switch sub.kind {
			case optErrorBody:
				if aggregate.kind != optErrorBody {
					aggregate.kind = optErrorBody
					aggregate.errorBody = sub.errorBody
				}
			case optErrorHooker:
				if aggregate.kind != optErrorBody {
					aggregate.kind = optErrorHooker
				}
			case optUnresolved:
				if aggregate.kind == optIrrelevant {
					aggregate.kind = optUnresolved
				}
			}
		}
	}
	if distinctKeys > 1 {
		aggregate.conflicted = true
	}
	return aggregate
}

// classifyKey produces a stable identifier for optResult used to count distinct
// classifications across branches. ErrorBody results with different E types
// count as distinct keys; conflicted is not part of the key.
func classifyKey(r optResult) string {
	switch r.kind {
	case optErrorBody:
		return "body:" + errorBodyKey(r.errorBody)
	case optErrorHooker:
		return "hooker"
	case optUnresolved:
		return "unresolved"
	default:
		return "irrelevant"
	}
}

// unwrapValueConversions strips SSA conversion wrappers that don't change the
// underlying value. Generic functions returning a parametric type often surface
// a *ssa.ChangeType between the body's *ssa.Call and the actual return value at
// the call site; classification needs to see through that.
func unwrapValueConversions(v ssa.Value) ssa.Value {
	for {
		switch u := v.(type) {
		case *ssa.ChangeType:
			v = u.X
		case *ssa.Convert:
			v = u.X
		default:
			return v
		}
	}
}

// errorBodyTypeArg extracts the E type parameter from a direct WithErrorBody[Reg, E] call.
func (g *tanukiTypeInfo) errorBodyTypeArg(call *ssa.Call) types.Type {
	sig := call.Call.Signature()
	if sig == nil || sig.Params().Len() < 1 {
		return nil
	}
	nt, ok := sig.Params().At(0).Type().(*types.Named)
	if !ok {
		return nil
	}
	args := nt.TypeArgs()
	if args == nil || args.Len() < 1 {
		return nil
	}
	return args.At(0)
}

// collectVariadicElements unwraps an SSA *ssa.Slice (the synthetic slice the
// compiler builds for variadic call sites) into the individual element values
// stored into the underlying array.
//
// Returns:
//   - elems: the values stored into the slice's backing array.
//     nil when v is not a *ssa.Slice over an Alloc.
//   - resolved: true when each backing array slot received exactly one Store.
//     false when the same slot (same constant index) received more than one
//     Store — typically a conditional overwrite like `opts[0] = x` that the
//     analyzer cannot decide statically. Callers should warn so the user
//     notices the runtime branch dependency.
func collectVariadicElements(v ssa.Value) (elems []ssa.Value, resolved bool) {
	slc, ok := v.(*ssa.Slice)
	if !ok {
		return nil, false
	}
	alloc, ok := slc.X.(*ssa.Alloc)
	if !ok {
		return nil, false
	}
	// IndexAddr instructions reaching the backing array can be rooted on the
	// Alloc (literal initialization) or on the resulting Slice value
	// (`opts[i] = ...` later in the function). Collect both so slot
	// overwrites surface regardless of how the user re-assigned.
	indexAddrs := collectBackingIndexAddrs(alloc, slc)
	if len(indexAddrs) == 0 {
		return nil, false
	}
	// Group Stores by the slot they target. Two *ssa.IndexAddr instructions
	// with the same constant index alias the same array slot, which is how
	// `opts[0] = ...` inside a conditional block surfaces.
	storesByIndex := map[string][]ssa.Value{}
	var ordering []string
	for _, idx := range indexAddrs {
		idxRefs := idx.Referrers()
		if idxRefs == nil {
			continue
		}
		key := indexKey(idx)
		for _, r := range *idxRefs {
			store, ok := r.(*ssa.Store)
			if !ok {
				continue
			}
			if _, seen := storesByIndex[key]; !seen {
				ordering = append(ordering, key)
			}
			storesByIndex[key] = append(storesByIndex[key], store.Val)
		}
	}
	resolved = true
	for _, key := range ordering {
		vals := storesByIndex[key]
		if len(vals) > 1 {
			resolved = false
		}
		elems = append(elems, vals...)
	}
	return elems, resolved
}

// indexKey identifies a backing-array slot reached by *ssa.IndexAddr. Constant
// indices share keys across IndexAddr instructions; non-constant indices fall
// back to the IndexAddr pointer identity so we never accidentally merge slots
// we cannot prove equal.
func indexKey(idx *ssa.IndexAddr) string {
	if c, ok := idx.Index.(*ssa.Const); ok && c.Value != nil {
		return "const:" + c.Value.ExactString()
	}
	return fmt.Sprintf("ptr:%p", idx)
}

// collectBackingIndexAddrs returns every *ssa.IndexAddr instruction reaching
// the same backing array, whether rooted on the Alloc (initial literal stores)
// or on the resulting Slice value (`opts[i] = ...` re-assignments).
func collectBackingIndexAddrs(alloc *ssa.Alloc, slc *ssa.Slice) []*ssa.IndexAddr {
	var out []*ssa.IndexAddr
	if refs := alloc.Referrers(); refs != nil {
		for _, ref := range *refs {
			if idx, ok := ref.(*ssa.IndexAddr); ok {
				out = append(out, idx)
			}
		}
	}
	if refs := slc.Referrers(); refs != nil {
		for _, ref := range *refs {
			if idx, ok := ref.(*ssa.IndexAddr); ok {
				out = append(out, idx)
			}
		}
	}
	return out
}

func (g *tanukiTypeInfo) errorBodyFromOption(v ssa.Value) types.Type {
	optCall, ok := v.(*ssa.Call)
	if !ok {
		return nil
	}
	callee, ok := optCall.Call.Value.(*ssa.Function)
	if !ok {
		return nil
	}
	if !isSameOriginFunc(callee.Object(), g.withErrorBodyObj) {
		return nil
	}
	// WithErrorBody[Reg, E](marshaler ErrorBodyMarshaler[E]) RouterOption[Reg]
	// The first parameter type is the named type ErrorBodyMarshaler[E].
	sig := optCall.Call.Signature()
	if sig.Params().Len() < 1 {
		return nil
	}
	p := sig.Params().At(0)
	nt, ok := p.Type().(*types.Named)
	if !ok {
		return nil
	}
	args := nt.TypeArgs()
	if args == nil || args.Len() < 1 {
		return nil
	}
	return args.At(0)
}

// isSameOriginFunc reports whether two function objects refer to the same
// generic function origin. SSA may surface an instantiated *types.Func for a
// call site, which is distinct from the origin found by package-scope lookup.
func isSameOriginFunc(a, b types.Object) bool {
	if a == nil || b == nil {
		return false
	}
	if a == b {
		return true
	}
	af, ok := a.(*types.Func)
	if !ok {
		return false
	}
	bf, ok := b.(*types.Func)
	if !ok {
		return false
	}
	return af.Origin() == bf.Origin()
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
	if len(args) != 5 {
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

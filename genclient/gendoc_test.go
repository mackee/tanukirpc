package genclient_test

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mackee/tanukirpc/genclient"
	"golang.org/x/tools/go/analysis/analysistest"
)

func TestGenerateTypeScriptClient(t *testing.T) {
	testdata := analysistest.TestData()
	results := analysistest.Run(t, testdata, genclient.TypeScriptClientGenerator, "./gendoctest")
	assertGoldenClientTS(t, filepath.Join(testdata, "gendoctest", "client.ts"), results)
}

func TestGenerateTypeScriptClientWithErrorBody(t *testing.T) {
	testdata := analysistest.TestData()
	results := analysistest.Run(t, testdata, genclient.TypeScriptClientGenerator, "./gendoctest_errbody")
	assertGoldenClientTS(t, filepath.Join(testdata, "gendoctest_errbody", "client.ts"), results)
}

func TestGenerateTypeScriptClientWithErrorBodyHelper(t *testing.T) {
	testdata := analysistest.TestData()
	results := analysistest.Run(t, testdata, genclient.TypeScriptClientGenerator, "./gendoctest_errbody_helper")
	assertGoldenClientTS(t, filepath.Join(testdata, "gendoctest_errbody_helper", "client.ts"), results)
}

func TestGenerateTypeScriptClientWithErrorBodyViaTypedHooker(t *testing.T) {
	testdata := analysistest.TestData()
	results := analysistest.Run(t, testdata, genclient.TypeScriptClientGenerator, "./gendoctest_errbody_hooker_typed")
	assertGoldenClientTS(t, filepath.Join(testdata, "gendoctest_errbody_hooker_typed", "client.ts"), results)
}

func TestGenerateTypeScriptClientAppliesTimeWhitelistInAllPositions(t *testing.T) {
	testdata := analysistest.TestData()
	results := analysistest.Run(t, testdata, genclient.TypeScriptClientGenerator, "./gendoctest_errbody_time_whitelist")
	assertGoldenClientTS(t, filepath.Join(testdata, "gendoctest_errbody_time_whitelist", "client.ts"), results)
}

func TestGenerateTypeScriptClientRefusesCustomMarshalJSON(t *testing.T) {
	testdata := analysistest.TestData()
	results := analysistest.Run(t, testdata, genclient.TypeScriptClientGenerator, "./gendoctest_errbody_marshaljson_fatal")
	assertGoldenClientTS(t, filepath.Join(testdata, "gendoctest_errbody_marshaljson_fatal", "client.ts"), results)
}

func TestGenerateTypeScriptClientRefusesInterfaceField(t *testing.T) {
	testdata := analysistest.TestData()
	results := analysistest.Run(t, testdata, genclient.TypeScriptClientGenerator, "./gendoctest_errbody_any_fatal")
	assertGoldenClientTS(t, filepath.Join(testdata, "gendoctest_errbody_any_fatal", "client.ts"), results)
}

func TestGenerateTypeScriptClientHonorsTstypeOverrideOnMarshalJSON(t *testing.T) {
	testdata := analysistest.TestData()
	results := analysistest.Run(t, testdata, genclient.TypeScriptClientGenerator, "./gendoctest_errbody_tstype_override")
	assertGoldenClientTS(t, filepath.Join(testdata, "gendoctest_errbody_tstype_override", "client.ts"), results)
}

func TestGenerateTypeScriptClientRefusesNoDiscriminator(t *testing.T) {
	testdata := analysistest.TestData()
	results := analysistest.Run(t, testdata, genclient.TypeScriptClientGenerator, "./gendoctest_errbody_no_discriminator_fatal")
	assertGoldenClientTS(t, filepath.Join(testdata, "gendoctest_errbody_no_discriminator_fatal", "client.ts"), results)
}

func TestGenerateTypeScriptClientRefusesNilableOnlyDiscriminators(t *testing.T) {
	testdata := analysistest.TestData()
	results := analysistest.Run(t, testdata, genclient.TypeScriptClientGenerator, "./gendoctest_errbody_nilable_only_fatal")
	assertGoldenClientTS(t, filepath.Join(testdata, "gendoctest_errbody_nilable_only_fatal", "client.ts"), results)
}

func TestGenerateTypeScriptClientRendersNestedEmptyStructAsObject(t *testing.T) {
	testdata := analysistest.TestData()
	results := analysistest.Run(t, testdata, genclient.TypeScriptClientGenerator, "./gendoctest_errbody_nested_empty_struct")
	assertGoldenClientTS(t, filepath.Join(testdata, "gendoctest_errbody_nested_empty_struct", "client.ts"), results)
}

func TestGenerateTypeScriptClientAcceptsEmptyStructAsOnlyDiscriminator(t *testing.T) {
	testdata := analysistest.TestData()
	results := analysistest.Run(t, testdata, genclient.TypeScriptClientGenerator, "./gendoctest_errbody_empty_struct_only_field")
	assertGoldenClientTS(t, filepath.Join(testdata, "gendoctest_errbody_empty_struct_only_field", "client.ts"), results)
}

func TestGenerateTypeScriptClientPeelsDefinedPointerFields(t *testing.T) {
	testdata := analysistest.TestData()
	results := analysistest.Run(t, testdata, genclient.TypeScriptClientGenerator, "./gendoctest_errbody_defined_pointer")
	assertGoldenClientTS(t, filepath.Join(testdata, "gendoctest_errbody_defined_pointer", "client.ts"), results)
}

func TestGenerateTypeScriptClientIgnoresPointerOnlyMarkerOnValueHooker(t *testing.T) {
	testdata := analysistest.TestData()
	results := analysistest.Run(t, testdata, genclient.TypeScriptClientGenerator, "./gendoctest_errbody_hooker_value_with_ptr_marker")
	assertGoldenClientTS(t, filepath.Join(testdata, "gendoctest_errbody_hooker_value_with_ptr_marker", "client.ts"), results)
}

func TestGenerateTypeScriptClientWithErrorBodyOverride(t *testing.T) {
	testdata := analysistest.TestData()
	results := analysistest.Run(t, testdata, genclient.TypeScriptClientGenerator, "./gendoctest_errbody_override")
	assertGoldenClientTS(t, filepath.Join(testdata, "gendoctest_errbody_override", "client.ts"), results)
}

func TestGenerateTypeScriptClientWithErrorBodySpread(t *testing.T) {
	testdata := analysistest.TestData()
	results := analysistest.Run(t, testdata, genclient.TypeScriptClientGenerator, "./gendoctest_errbody_spread")
	assertGoldenClientTS(t, filepath.Join(testdata, "gendoctest_errbody_spread", "client.ts"), results)
}

func TestGenerateTypeScriptClientWithErrorBodyWrapper(t *testing.T) {
	testdata := analysistest.TestData()
	results := analysistest.Run(t, testdata, genclient.TypeScriptClientGenerator, "./gendoctest_errbody_wrapper")
	assertGoldenClientTS(t, filepath.Join(testdata, "gendoctest_errbody_wrapper", "client.ts"), results)
}

func TestGenerateTypeScriptClientWithUnresolvedOptionFallsBack(t *testing.T) {
	testdata := analysistest.TestData()
	results := analysistest.Run(t, testdata, genclient.TypeScriptClientGenerator, "./gendoctest_errbody_warn")
	assertGoldenClientTS(t, filepath.Join(testdata, "gendoctest_errbody_warn", "client.ts"), results)
}

func TestGenerateTypeScriptClientWithUnresolvedSpread(t *testing.T) {
	testdata := analysistest.TestData()
	results := analysistest.Run(t, testdata, genclient.TypeScriptClientGenerator, "./gendoctest_errbody_warn_spread")
	assertGoldenClientTS(t, filepath.Join(testdata, "gendoctest_errbody_warn_spread", "client.ts"), results)
}

func TestGenerateTypeScriptClientWithBranchingRouterFactory(t *testing.T) {
	testdata := analysistest.TestData()
	results := analysistest.Run(t, testdata, genclient.TypeScriptClientGenerator, "./gendoctest_errbody_branch_router")
	assertGoldenClientTS(t, filepath.Join(testdata, "gendoctest_errbody_branch_router", "client.ts"), results)
}

func TestGenerateTypeScriptClientWithBranchingOptionWrapper(t *testing.T) {
	testdata := analysistest.TestData()
	results := analysistest.Run(t, testdata, genclient.TypeScriptClientGenerator, "./gendoctest_errbody_branch_option")
	assertGoldenClientTS(t, filepath.Join(testdata, "gendoctest_errbody_branch_option", "client.ts"), results)
}

func TestGenerateTypeScriptClientWithGenericFactory(t *testing.T) {
	testdata := analysistest.TestData()
	results := analysistest.Run(t, testdata, genclient.TypeScriptClientGenerator, "./gendoctest_errbody_generic_factory")
	assertGoldenClientTS(t, filepath.Join(testdata, "gendoctest_errbody_generic_factory", "client.ts"), results)
}

func TestGenerateTypeScriptClientHandlesGenericSpreadHelper(t *testing.T) {
	testdata := analysistest.TestData()
	results := analysistest.Run(t, testdata, genclient.TypeScriptClientGenerator, "./gendoctest_errbody_generic_spread")
	assertGoldenClientTS(t, filepath.Join(testdata, "gendoctest_errbody_generic_spread", "client.ts"), results)
}

func TestGenerateTypeScriptClientRejectsMultipleAnalyzeTarget(t *testing.T) {
	testdata := analysistest.TestData()
	results := analysistest.Run(t, testdata, genclient.TypeScriptClientGenerator, "./gendoctest_errbody_multi")
	assertGoldenClientTS(t, filepath.Join(testdata, "gendoctest_errbody_multi", "client.ts"), results)
}

func TestGenerateTypeScriptClientWarnsOnDivergentSpreadHelper(t *testing.T) {
	testdata := analysistest.TestData()
	results := analysistest.Run(t, testdata, genclient.TypeScriptClientGenerator, "./gendoctest_errbody_spread_branch")
	assertGoldenClientTS(t, filepath.Join(testdata, "gendoctest_errbody_spread_branch", "client.ts"), results)
}

func TestGenerateTypeScriptClientWarnsOnUntaggedErrorBodyField(t *testing.T) {
	testdata := analysistest.TestData()
	results := analysistest.Run(t, testdata, genclient.TypeScriptClientGenerator, "./gendoctest_errbody_untagged")
	assertGoldenClientTS(t, filepath.Join(testdata, "gendoctest_errbody_untagged", "client.ts"), results)
}

func TestGenerateTypeScriptClientWarnsOnPointerErrorBody(t *testing.T) {
	testdata := analysistest.TestData()
	results := analysistest.Run(t, testdata, genclient.TypeScriptClientGenerator, "./gendoctest_errbody_ptr")
	assertGoldenClientTS(t, filepath.Join(testdata, "gendoctest_errbody_ptr", "client.ts"), results)
}

func TestGenerateTypeScriptClientWarnsOnSliceOverwrite(t *testing.T) {
	testdata := analysistest.TestData()
	results := analysistest.Run(t, testdata, genclient.TypeScriptClientGenerator, "./gendoctest_errbody_slice_overwrite")
	assertGoldenClientTS(t, filepath.Join(testdata, "gendoctest_errbody_slice_overwrite", "client.ts"), results)
}

func TestGenerateTypeScriptClientWarnsOnPhiRouter(t *testing.T) {
	testdata := analysistest.TestData()
	results := analysistest.Run(t, testdata, genclient.TypeScriptClientGenerator, "./gendoctest_errbody_phi")
	assertGoldenClientTS(t, filepath.Join(testdata, "gendoctest_errbody_phi", "client.ts"), results)
}

func TestGenerateTypeScriptClientFollowsWithChain(t *testing.T) {
	testdata := analysistest.TestData()
	results := analysistest.Run(t, testdata, genclient.TypeScriptClientGenerator, "./gendoctest_errbody_with_chain")
	assertGoldenClientTS(t, filepath.Join(testdata, "gendoctest_errbody_with_chain", "client.ts"), results)
}

func TestGenerateTypeScriptClientWarnsOnCrossPkgFactory(t *testing.T) {
	testdata := analysistest.TestData()
	results := analysistest.Run(t, testdata, genclient.TypeScriptClientGenerator, "./gendoctest_errbody_cross_pkg")
	assertGoldenClientTS(t, filepath.Join(testdata, "gendoctest_errbody_cross_pkg", "client.ts"), results)
}

func TestGenerateTypeScriptClientHandlesEmbeddedErrorBody(t *testing.T) {
	testdata := analysistest.TestData()
	results := analysistest.Run(t, testdata, genclient.TypeScriptClientGenerator, "./gendoctest_errbody_embedded")
	assertGoldenClientTS(t, filepath.Join(testdata, "gendoctest_errbody_embedded", "client.ts"), results)
}

func TestGenerateTypeScriptClientWarnsOnAnonPointerErrorBody(t *testing.T) {
	testdata := analysistest.TestData()
	results := analysistest.Run(t, testdata, genclient.TypeScriptClientGenerator, "./gendoctest_errbody_anon_ptr")
	assertGoldenClientTS(t, filepath.Join(testdata, "gendoctest_errbody_anon_ptr", "client.ts"), results)
}

func TestGenerateTypeScriptClientRefusesEmptyErrorBody(t *testing.T) {
	testdata := analysistest.TestData()
	results := analysistest.Run(t, testdata, genclient.TypeScriptClientGenerator, "./gendoctest_errbody_empty")
	assertGoldenClientTS(t, filepath.Join(testdata, "gendoctest_errbody_empty", "client.ts"), results)
}

func TestGenerateTypeScriptClientRefusesZeroFieldErrorBody(t *testing.T) {
	testdata := analysistest.TestData()
	results := analysistest.Run(t, testdata, genclient.TypeScriptClientGenerator, "./gendoctest_errbody_struct_empty")
	assertGoldenClientTS(t, filepath.Join(testdata, "gendoctest_errbody_struct_empty", "client.ts"), results)
}

func TestGenerateTypeScriptClientRefusesDashOnlyErrorBody(t *testing.T) {
	testdata := analysistest.TestData()
	results := analysistest.Run(t, testdata, genclient.TypeScriptClientGenerator, "./gendoctest_errbody_dash_only")
	assertGoldenClientTS(t, filepath.Join(testdata, "gendoctest_errbody_dash_only", "client.ts"), results)
}

func TestGenerateTypeScriptClientRefusesEmbeddedOnlyErrorBody(t *testing.T) {
	testdata := analysistest.TestData()
	results := analysistest.Run(t, testdata, genclient.TypeScriptClientGenerator, "./gendoctest_errbody_embedded_only")
	assertGoldenClientTS(t, filepath.Join(testdata, "gendoctest_errbody_embedded_only", "client.ts"), results)
}

func TestGenerateTypeScriptClientRefusesUnexportedOnlyErrorBody(t *testing.T) {
	testdata := analysistest.TestData()
	results := analysistest.Run(t, testdata, genclient.TypeScriptClientGenerator, "./gendoctest_errbody_unexported_only")
	assertGoldenClientTS(t, filepath.Join(testdata, "gendoctest_errbody_unexported_only", "client.ts"), results)
}

func TestGenerateTypeScriptClientWarnsOnUnexportedTaggedField(t *testing.T) {
	testdata := analysistest.TestData()
	results := analysistest.Run(t, testdata, genclient.TypeScriptClientGenerator, "./gendoctest_errbody_unexported_mix")
	assertGoldenClientTS(t, filepath.Join(testdata, "gendoctest_errbody_unexported_mix", "client.ts"), results)
}

// TestGoldenClientsTypeCheck runs the TypeScript compiler over every
// non-empty client.ts golden so we catch generation-time regressions that
// produce invalid TypeScript — including issues a linter would flag (the
// `useLiteralKeys` rule's bracket-notation suggestion, undeclared
// identifiers, etc.). Skipped when tsc isn't on PATH so the rest of the
// test suite stays runnable.
func TestGoldenClientsTypeCheck(t *testing.T) {
	tscPath, err := exec.LookPath("tsc")
	if err != nil {
		t.Skipf("tsc not on PATH: %v", err)
	}
	testdata := analysistest.TestData()
	matches, err := filepath.Glob(filepath.Join(testdata, "gendoctest*", "client.ts"))
	if err != nil {
		t.Fatalf("glob client.ts goldens: %v", err)
	}
	if len(matches) == 0 {
		t.Fatalf("no client.ts goldens found under %s", testdata)
	}
	for _, p := range matches {
		p := p
		name := filepath.Base(filepath.Dir(p))
		t.Run(name, func(t *testing.T) {
			info, err := os.Stat(p)
			if err != nil {
				t.Fatalf("stat %s: %v", p, err)
			}
			// Fatal-case goldens are intentionally empty placeholders;
			// there's nothing for tsc to check.
			if info.Size() == 0 {
				t.Skip("empty fatal-case golden")
			}
			cmd := exec.Command(tscPath,
				"--noEmit",
				"--strict",
				"--target", "es2020",
				"--module", "esnext",
				"--moduleResolution", "bundler",
				"--lib", "es2020,dom",
				"--skipLibCheck",
				p,
			)
			out, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("tsc rejected %s:\n%s", p, strings.TrimSpace(string(out)))
			}
		})
	}
}

func assertGoldenClientTS(t *testing.T, goldenPath string, results []*analysistest.Result) {
	t.Helper()
	if len(results) != 1 {
		t.Fatalf("expected exactly 1 analysis result, got %d", len(results))
	}
	got, ok := results[0].Result.(*bytes.Buffer)
	if !ok {
		t.Fatalf("expected *bytes.Buffer result, got %T", results[0].Result)
	}
	want, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Fatalf("failed to read golden file %s: %v", goldenPath, err)
	}
	if got.String() != string(want) {
		t.Fatalf("generated TypeScript client does not match golden file %s.\n"+
			"Run `go generate ./...` to regenerate.\n\n--- want ---\n%s\n--- got ---\n%s",
			goldenPath, string(want), got.String())
	}
}

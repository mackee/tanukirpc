package genclient_test

import (
	"bytes"
	"os"
	"path/filepath"
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

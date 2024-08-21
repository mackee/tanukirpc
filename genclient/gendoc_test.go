package genclient_test

import (
	"testing"

	"github.com/mackee/tanukirpc/genclient"
	"golang.org/x/tools/go/analysis/analysistest"
)

func TestGenerateTypeScriptClient(t *testing.T) {
	testdata := analysistest.TestData()
	analysistest.Run(t, testdata, genclient.TypeScriptClientGenerator, "./gendoctest")
}

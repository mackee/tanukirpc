package genclient

import (
	"encoding/json"
	"fmt"
	"os"

	"golang.org/x/tools/go/analysis"
)

var ShowPathAnalyzer = &analysis.Analyzer{
	Name: "showpaths",
	Doc:  "show paths",
	Run:  runShowPaths,
	Requires: []*analysis.Analyzer{
		Analyzer,
	},
}

func runShowPaths(pass *analysis.Pass) (any, error) {
	result := pass.ResultOf[Analyzer].(*AnalyzerResult)
	rps := result.RoutePaths

	rpps := make([]showPathPath, 0, len(rps))
	for _, rp := range rps {
		rpps = append(rpps, showPathPath{
			Method: rp.Method(),
			Path:   rp.Path(),
		})
	}
	jsonRet := showPathResult{
		Paths: rpps,
	}
	if err := json.NewEncoder(os.Stdout).Encode(jsonRet); err != nil {
		return nil, fmt.Errorf("failed to encode json: %w", err)
	}

	return nil, nil
}

type showPathResult struct {
	Paths []showPathPath `json:"paths"`
}

type showPathPath struct {
	Method string `json:"method"`
	Path   string `json:"path"`
}

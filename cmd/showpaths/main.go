package main

import (
	"github.com/mackee/tanukirpc/genclient"
	"golang.org/x/tools/go/analysis/singlechecker"
)

func main() {
	singlechecker.Main(genclient.ShowPathAnalyzer)
}

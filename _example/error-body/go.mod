module github.com/mackee/tanukirpc/_example/error-body

go 1.26.0

replace github.com/mackee/tanukirpc => ../..

require github.com/mackee/tanukirpc v0.9.0

require (
	github.com/ajg/form v1.7.1 // indirect
	github.com/cpuguy83/go-md2man/v2 v2.0.7 // indirect
	github.com/fsnotify/fsnotify v1.9.0 // indirect
	github.com/gabriel-vasile/mimetype v1.4.13 // indirect
	github.com/go-chi/chi/v5 v5.2.5 // indirect
	github.com/go-chi/render v1.0.3 // indirect
	github.com/go-playground/locales v0.14.1 // indirect
	github.com/go-playground/universal-translator v0.18.1 // indirect
	github.com/go-playground/validator/v10 v10.30.1 // indirect
	github.com/gostaticanalysis/analysisutil v0.7.1 // indirect
	github.com/gostaticanalysis/comment v1.5.0 // indirect
	github.com/hetiansu5/urlquery v1.2.7 // indirect
	github.com/leodido/go-urn v1.4.0 // indirect
	github.com/russross/blackfriday/v2 v2.1.0 // indirect
	github.com/urfave/cli/v2 v2.27.7 // indirect
	github.com/xrash/smetrics v0.0.0-20250705151800-55b8f293f342 // indirect
	golang.org/x/crypto v0.49.0 // indirect
	golang.org/x/mod v0.34.0 // indirect
	golang.org/x/sync v0.20.0 // indirect
	golang.org/x/sys v0.42.0 // indirect
	golang.org/x/text v0.35.0 // indirect
	golang.org/x/tools v0.42.0 // indirect
	golang.org/x/tools/go/expect v0.1.1-deprecated // indirect
)

tool (
	github.com/mackee/tanukirpc/cmd/gentypescript
	github.com/mackee/tanukirpc/cmd/tanukiup
)

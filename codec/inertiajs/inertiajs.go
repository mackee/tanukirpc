package inertiajs

import (
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"log/slog"
	"net/http"
	"reflect"
	"strings"

	"github.com/mackee/tanukirpc"
)

const (
	inertiaHeader        = "X-Inertia"
	inertiaHeaderValue   = "true"
	inertiaVaryHeader    = "Vary"
	inertiaContentType   = "application/json"
	inertiaHTMLMediaType = "text/html; charset=utf-8"
)

// Page is the response shape handlers return for Inertia.js routes.
type Page[P any] struct {
	Component        string `json:"component"`
	Props            P      `json:"props"`
	URL              string `json:"url,omitempty"`
	Version          any    `json:"version,omitempty"`
	EncryptHistory   bool   `json:"encryptHistory,omitempty"`
	ClearHistory     bool   `json:"clearHistory,omitempty"`
	PreserveFragment bool   `json:"preserveFragment,omitempty"`
}

// PageObject is the normalized Inertia page object sent to clients and templates.
type PageObject struct {
	Component        string         `json:"component"`
	Props            map[string]any `json:"props"`
	URL              string         `json:"url"`
	Version          any            `json:"version,omitempty"`
	EncryptHistory   bool           `json:"encryptHistory,omitempty"`
	ClearHistory     bool           `json:"clearHistory,omitempty"`
	PreserveFragment bool           `json:"preserveFragment,omitempty"`
}

type pageResponse interface {
	inertiaPage() rawPage
}

type rawPage struct {
	Component        string
	Props            any
	URL              string
	Version          any
	EncryptHistory   bool
	ClearHistory     bool
	PreserveFragment bool
}

func (p Page[P]) inertiaPage() rawPage {
	return rawPage{
		Component:        p.Component,
		Props:            p.Props,
		URL:              p.URL,
		Version:          p.Version,
		EncryptHistory:   p.EncryptHistory,
		ClearHistory:     p.ClearHistory,
		PreserveFragment: p.PreserveFragment,
	}
}

func (p PageObject) inertiaPage() rawPage {
	return rawPage{
		Component:        p.Component,
		Props:            p.Props,
		URL:              p.URL,
		Version:          p.Version,
		EncryptHistory:   p.EncryptHistory,
		ClearHistory:     p.ClearHistory,
		PreserveFragment: p.PreserveFragment,
	}
}

type pageOptions struct {
	url              string
	version          any
	encryptHistory   bool
	clearHistory     bool
	preserveFragment bool
}

// PageOption configures a Page returned by Render.
type PageOption func(*pageOptions)

// WithPageURL sets the page URL explicitly. When omitted, the codec uses the request URI.
func WithPageURL(url string) PageOption {
	return func(o *pageOptions) {
		o.url = url
	}
}

// WithPageVersion sets the page asset version explicitly.
func WithPageVersion(version any) PageOption {
	return func(o *pageOptions) {
		o.version = version
	}
}

// WithEncryptHistory configures Inertia encrypted history for the page.
func WithEncryptHistory(enabled bool) PageOption {
	return func(o *pageOptions) {
		o.encryptHistory = enabled
	}
}

// WithClearHistory configures whether Inertia should clear browser history.
func WithClearHistory(enabled bool) PageOption {
	return func(o *pageOptions) {
		o.clearHistory = enabled
	}
}

// WithPreserveFragment configures whether Inertia should preserve the URL fragment.
func WithPreserveFragment(enabled bool) PageOption {
	return func(o *pageOptions) {
		o.preserveFragment = enabled
	}
}

// Render builds a typed Inertia.js page response.
func Render[P any](component string, props P, opts ...PageOption) Page[P] {
	options := pageOptions{}
	for _, opt := range opts {
		opt(&options)
	}
	return Page[P]{
		Component:        component,
		Props:            props,
		URL:              options.url,
		Version:          options.version,
		EncryptHistory:   options.encryptHistory,
		ClearHistory:     options.clearHistory,
		PreserveFragment: options.preserveFragment,
	}
}

// TemplateData is passed to the initial HTML shell template.
type TemplateData struct {
	Page     PageObject
	PageJSON template.JS
	Request  *http.Request
}

type codecOptions struct {
	templateName     string
	assetVersion     any
	assetVersionFunc func(*http.Request) (any, error)
}

// Option configures a Codec.
type Option func(*codecOptions)

// WithTemplateName selects the template name used for the initial HTML shell.
func WithTemplateName(name string) Option {
	return func(o *codecOptions) {
		o.templateName = name
	}
}

// WithAssetVersion sets the asset version included in Inertia page objects.
func WithAssetVersion(version any) Option {
	return func(o *codecOptions) {
		o.assetVersion = version
	}
}

// WithAssetVersionFunc sets a request-aware asset version provider.
func WithAssetVersionFunc(fn func(*http.Request) (any, error)) Option {
	return func(o *codecOptions) {
		o.assetVersionFunc = fn
	}
}

// Codec renders Inertia.js page responses.
type Codec struct {
	template *template.Template
	options  codecOptions
}

// New creates an Inertia.js codec.
func New(tmpl *template.Template, opts ...Option) *Codec {
	options := codecOptions{}
	for _, opt := range opts {
		opt(&options)
	}
	return &Codec{
		template: tmpl,
		options:  options,
	}
}

func (c *Codec) Name() string {
	return "inertiajs"
}

func (c *Codec) Encode(w http.ResponseWriter, r *http.Request, v any) error {
	return c.WritePage(w, r, v, 0)
}

// WritePage writes an Inertia page with an explicit HTTP status.
func (c *Codec) WritePage(w http.ResponseWriter, r *http.Request, v any, status int) error {
	if v == nil || c.isPointerResponse(v) {
		return tanukirpc.ErrResponseNotSupportedAtThisCodec
	}
	page, ok := v.(pageResponse)
	if !ok {
		return tanukirpc.ErrResponseNotSupportedAtThisCodec
	}
	po, err := c.normalize(r, page.inertiaPage())
	if err != nil {
		return err
	}
	return c.writePageObject(w, r, po, status)
}

func (c *Codec) Decode(r *http.Request, v any) error {
	return tanukirpc.ErrRequestNotSupportedAtThisCodec
}

func (c *Codec) normalize(r *http.Request, page rawPage) (PageObject, error) {
	if page.Component == "" {
		return PageObject{}, errors.New("inertia component is required")
	}
	props, err := c.normalizeProps(page.Props)
	if err != nil {
		return PageObject{}, err
	}
	url := page.URL
	if url == "" {
		url = r.URL.RequestURI()
	}
	version := page.Version
	if version == nil {
		version, err = c.assetVersion(r)
		if err != nil {
			return PageObject{}, err
		}
	}
	return PageObject{
		Component:        page.Component,
		Props:            props,
		URL:              url,
		Version:          version,
		EncryptHistory:   page.EncryptHistory,
		ClearHistory:     page.ClearHistory,
		PreserveFragment: page.PreserveFragment,
	}, nil
}

func (c *Codec) assetVersion(r *http.Request) (any, error) {
	if c == nil {
		return nil, nil
	}
	if c.options.assetVersionFunc != nil {
		return c.options.assetVersionFunc(r)
	}
	return c.options.assetVersion, nil
}

func (c *Codec) writePageObject(w http.ResponseWriter, r *http.Request, page PageObject, status int) error {
	c.appendVary(w.Header())
	if c.isRequest(r) {
		w.Header().Set("Content-Type", inertiaContentType)
		w.Header().Set(inertiaHeader, inertiaHeaderValue)
		if status != 0 {
			w.WriteHeader(status)
		}
		return json.NewEncoder(w).Encode(page)
	}
	if c == nil || c.template == nil {
		return errors.New("inertia template is required for non X-Inertia requests")
	}
	pageJSON, err := json.Marshal(page)
	if err != nil {
		return fmt.Errorf("failed to marshal inertia page: %w", err)
	}
	w.Header().Set("Content-Type", inertiaHTMLMediaType)
	if status != 0 {
		w.WriteHeader(status)
	}
	data := TemplateData{
		Page:     page,
		PageJSON: template.JS(pageJSON),
		Request:  r,
	}
	if c.options.templateName != "" {
		return c.template.ExecuteTemplate(w, c.options.templateName, data)
	}
	return c.template.Execute(w, data)
}

func (c *Codec) normalizeProps(props any) (map[string]any, error) {
	raw, err := json.Marshal(props)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal inertia props: %w", err)
	}
	var values map[string]any
	if err := json.Unmarshal(raw, &values); err != nil {
		return nil, fmt.Errorf("inertia props must encode to a JSON object: %w", err)
	}
	if values == nil {
		return nil, errors.New("inertia props must encode to a JSON object")
	}
	if _, ok := values["errors"]; !ok {
		values["errors"] = map[string]any{}
	}
	return values, nil
}

func (*Codec) isPointerResponse(v any) bool {
	rv := reflect.ValueOf(v)
	return rv.IsValid() && rv.Kind() == reflect.Pointer
}

func (*Codec) isRequest(r *http.Request) bool {
	return r.Header.Get(inertiaHeader) == inertiaHeaderValue
}

func (c *Codec) shouldHandleError(req *http.Request) bool {
	if c.isRequest(req) {
		return true
	}
	for _, accept := range strings.Split(req.Header.Get("Accept"), ",") {
		if strings.TrimSpace(strings.Split(accept, ";")[0]) == "text/html" {
			return true
		}
	}
	return false
}

func (*Codec) appendVary(header http.Header) {
	current := header.Get(inertiaVaryHeader)
	if current == "" {
		header.Set(inertiaVaryHeader, inertiaHeader)
		return
	}
	for _, part := range strings.Split(current, ",") {
		if strings.EqualFold(strings.TrimSpace(part), inertiaHeader) {
			return
		}
	}
	header.Set(inertiaVaryHeader, current+", "+inertiaHeader)
}

// ErrorPageFunc builds an Inertia error page for the given error and status.
type ErrorPageFunc func(req *http.Request, err error, status int) Page[map[string]any]

// ErrorHookerOption configures an Inertia-aware error hooker.
type ErrorHookerOption func(*inertiaErrorHooker)

// WithFallbackErrorHooker sets the error hooker used for non-Inertia errors or write failures.
func WithFallbackErrorHooker(fallback tanukirpc.ErrorHooker) ErrorHookerOption {
	return func(h *inertiaErrorHooker) {
		h.fallback = fallback
	}
}

// NewErrorHooker creates an error hooker that can render Inertia error pages.
func NewErrorHooker(ic *Codec, errorPage ErrorPageFunc, opts ...ErrorHookerOption) tanukirpc.ErrorHooker {
	h := &inertiaErrorHooker{
		codec:     ic,
		errorPage: errorPage,
		fallback:  defaultErrorHooker{},
	}
	for _, opt := range opts {
		opt(h)
	}
	return h
}

type inertiaErrorHooker struct {
	codec     *Codec
	errorPage ErrorPageFunc
	fallback  tanukirpc.ErrorHooker
}

func (h *inertiaErrorHooker) OnError(w http.ResponseWriter, req *http.Request, logger *slog.Logger, codec tanukirpc.Codec, err error) {
	if h.codec == nil || !h.codec.shouldHandleError(req) {
		h.fallback.OnError(w, req, logger, codec, err)
		return
	}
	var redirect tanukirpc.ErrorWithRedirect
	if errors.As(err, &redirect) {
		http.Redirect(w, req, redirect.Redirect(), redirect.Status())
		return
	}
	status := http.StatusInternalServerError
	var withStatus tanukirpc.ErrorWithStatus
	if errors.As(err, &withStatus) {
		status = withStatus.Status()
	} else {
		logger.ErrorContext(req.Context(), "ocurred internal server error", slog.Any("error", err))
	}
	errorPage := h.errorPage
	if errorPage == nil {
		errorPage = h.defaultErrorPage
	}
	if writeErr := h.codec.WritePage(w, req, errorPage(req, err, status), status); writeErr != nil {
		logger.ErrorContext(req.Context(), "failed to encode inertia error response", slog.Any("error", writeErr))
		h.fallback.OnError(w, req, logger, codec, err)
	}
}

func (*inertiaErrorHooker) defaultErrorPage(_ *http.Request, err error, status int) Page[map[string]any] {
	return Render("Error", map[string]any{
		"status":  status,
		"message": err.Error(),
	})
}

type defaultErrorHooker struct{}

func (defaultErrorHooker) OnError(w http.ResponseWriter, req *http.Request, logger *slog.Logger, codec tanukirpc.Codec, err error) {
	var redirect tanukirpc.ErrorWithRedirect
	if errors.As(err, &redirect) {
		http.Redirect(w, req, redirect.Redirect(), redirect.Status())
		return
	}
	var withStatus tanukirpc.ErrorWithStatus
	if errors.As(err, &withStatus) {
		w.WriteHeader(withStatus.Status())
	} else {
		w.WriteHeader(http.StatusInternalServerError)
		logger.ErrorContext(req.Context(), "ocurred internal server error", slog.Any("error", err))
	}
	if encodeErr := codec.Encode(w, req, tanukirpc.ErrorMessage{Error: tanukirpc.ErrorBody{Message: err.Error()}}); encodeErr != nil {
		logger.ErrorContext(req.Context(), "failed to encode error response", slog.Any("error", encodeErr))
	}
}

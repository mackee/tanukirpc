package tanukiup

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"math/rand/v2"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/go-chi/chi/v5"
)

const (
	defaultLogLevel             = slog.LevelInfo
	generateDetectTargetFileExt = ".go"
	buildOutPathPlaceholder     = "{outpath}"
)

var (
	defaultFileExts     = []string{".go"}
	defaultDirs         = []string{"./"}
	defaultIgnoreDirs   = []string{".git"}
	defaultBuildCommand = []string{"go", "build", "-o", "{outpath}", "./"}
	defaultExecCommand  = []string{"{outpath}"}
	whitelistGenerate   = map[string]struct{}{
		"github.com/mackee/tanukirpc/cmd/gentypescript": {},
	}
)

type optionArgs struct {
	fileExts       []string
	dirs           []string
	ignoreDirs     []string
	buildCommand   []string
	execCommand    []string
	addr           string
	tempDir        string
	baseDir        string
	catchAllTarget string
	logLevel       slog.Level
}

func newDefaultOptionArgs() *optionArgs {
	tempDir := os.TempDir()
	baseDir := "."
	return &optionArgs{
		fileExts:     defaultFileExts,
		dirs:         defaultDirs,
		ignoreDirs:   defaultIgnoreDirs,
		buildCommand: defaultBuildCommand,
		execCommand:  defaultExecCommand,
		tempDir:      tempDir,
		baseDir:      baseDir,
		logLevel:     defaultLogLevel,
	}
}

type Option func(*optionArgs)

type Options []Option

func (o Options) apply(args *optionArgs) {
	for _, opt := range o {
		opt(args)
	}
}

func WithFileExts(fileExts []string) Option {
	return func(args *optionArgs) {
		args.fileExts = fileExts
	}
}

func WithDirs(dirs []string) Option {
	return func(args *optionArgs) {
		args.dirs = dirs
	}
}

func WithIgnoreDirs(ignoreDirs []string) Option {
	return func(args *optionArgs) {
		args.ignoreDirs = ignoreDirs
	}
}

func WithAddr(addr string) Option {
	return func(args *optionArgs) {
		args.addr = addr
	}
}

func WithLogLevel(logLevel slog.Level) Option {
	return func(args *optionArgs) {
		args.logLevel = logLevel
	}
}

func WithBuildCommand(command []string) Option {
	return func(args *optionArgs) {
		args.buildCommand = command
	}
}

func WithExecCommand(command []string) Option {
	return func(args *optionArgs) {
		args.execCommand = command
	}
}

func WithTempDir(tempDir string) Option {
	return func(args *optionArgs) {
		args.tempDir = tempDir
	}
}

func WithBaseDir(baseDir string) Option {
	return func(args *optionArgs) {
		args.baseDir = baseDir
	}
}

func WithCatchAllTarget(target string) Option {
	return func(args *optionArgs) {
		args.catchAllTarget = target
	}
}

func Run(ctx context.Context, options ...Option) error {
	args := newDefaultOptionArgs()
	Options(options).apply(args)

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: args.logLevel,
	}))
	slog.SetDefault(logger)

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return fmt.Errorf("failed to create watcher: %w", err)
	}
	defer watcher.Close()

	errChan := make(chan error)
	defer close(errChan)
	restartChan := make(chan struct{})
	defer close(restartChan)
	go func() {
		skipStart := false
		for {
			cmdCtx, cancel := context.WithCancel(ctx)
			if !skipStart {
				go func() {
					if err := runGenerator(ctx); err != nil {
						var exitError *exec.ExitError
						if !errors.Is(err, context.Canceled) &&
							!errors.As(err, &exitError) &&
							exitError.ExitCode() != -1 {
							slog.ErrorContext(ctx, "failed to generate command", slog.Any("error", err))
							errChan <- err
						}
						return
					}
					if err := startCmd(cmdCtx, args); err != nil {
						var exitError *exec.ExitError
						if !errors.Is(err, context.Canceled) &&
							!errors.As(err, &exitError) &&
							exitError.ExitCode() != -1 {
							slog.ErrorContext(ctx, "failed to start command", slog.Any("error", err))
							errChan <- err
						}
					}
				}()
			}
			select {
			case <-ctx.Done():
				cancel()
				return
			case <-restartChan:
				cancel()
				skipStart = false
			case <-errChan:
				cancel()
				skipStart = true
			}
		}
	}()

	extMap := make(map[string]struct{})
	for _, ext := range args.fileExts {
		extMap[ext] = struct{}{}
	}

	go func() {
		for {
			select {
			case event, ok := <-watcher.Events:
				if !ok {
					slog.InfoContext(ctx, "watcher is closed")
					return
				}
				slog.DebugContext(ctx, "event", slog.Any("event", event))
				if event.Has(fsnotify.Write) || event.Has(fsnotify.Create) {
					if _, ok := extMap[filepath.Ext(event.Name)]; !ok {
						continue
					}
					slog.InfoContext(ctx, "modified file", slog.String("filename", event.Name))
					restartChan <- struct{}{}
				}
			case err, ok := <-watcher.Errors:
				if !ok {
					slog.InfoContext(ctx, "watcher is closed")
					return
				}
				slog.ErrorContext(ctx, "watcher error", slog.Any("error", err))
			}
		}
	}()

	ignoreDirsMap := make(map[string]struct{})
	for _, dir := range args.ignoreDirs {
		dir := filepath.Join(args.baseDir, dir)
		ignoreDirsMap[dir] = struct{}{}
	}

	for _, dir := range args.dirs {
		dir := filepath.Join(args.baseDir, dir)
		if noRecursive := strings.TrimSuffix(dir, "..."); noRecursive != dir {
			stat, err := os.Stat(noRecursive)
			if err != nil {
				return fmt.Errorf("failed to get directory info: %w", err)
			}
			if !stat.IsDir() {
				return fmt.Errorf("not a directory: %s", noRecursive)
			}
			if err := filepath.WalkDir(noRecursive, walkDirFunc(ctx, ignoreDirsMap, watcher)); err != nil {
				return fmt.Errorf("failed to walk directory: %w", err)
			}
			continue
		}
		if err := walkDirFunc(ctx, ignoreDirsMap, watcher)(dir, nil, nil); err != nil {
			return fmt.Errorf("failed to walk directory: %w", err)
		}
	}

	<-ctx.Done()
	return nil
}

type isDirer interface {
	IsDir() bool
}

func walkDirFunc(ctx context.Context, ignoreDirsMap map[string]struct{}, watcher *fsnotify.Watcher) fs.WalkDirFunc {
	return func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return fmt.Errorf("failed to walk directory: %w", err)
		}
		if d != nil && !d.IsDir() {
			return nil
		}
		drs, err := os.ReadDir(p)
		if err != nil {
			return fmt.Errorf("failed to read directory: %w", err)
		}
		for _, f := range drs {
			if f.IsDir() {
				continue
			}
			if filepath.Ext(f.Name()) == generateDetectTargetFileExt {
				if err := searchGenerate(ctx, filepath.Join(p, f.Name())); err != nil {
					return fmt.Errorf("failed to search generate: %w", err)
				}
			}
		}

		if _, ok := ignoreDirsMap[p]; ok {
			return filepath.SkipDir
		}

		if err := watcher.Add(p); err != nil {
			return fmt.Errorf("failed to add directory to watcher: %w", err)
		}
		slog.InfoContext(ctx, "watching directory", slog.String("directory", p))
		return nil
	}
}

const (
	defaultTanukiupUDSPathEnv = "TANUKIUP_UDS_PATH"
)

func startCmd(ctx context.Context, args *optionArgs) error {
	fname := strconv.FormatUint(rand.Uint64(), 10)
	outpath := filepath.Join(args.tempDir, fname)
	buildCommand := make([]string, 0, len(args.buildCommand))
	for _, bc := range args.buildCommand {
		if bc == buildOutPathPlaceholder {
			buildCommand = append(buildCommand, outpath)
		} else {
			buildCommand = append(buildCommand, bc)
		}
	}
	slog.InfoContext(ctx, "building command", slog.Any("command", buildCommand))
	bcmd := exec.CommandContext(ctx, buildCommand[0], buildCommand[1:]...)
	bcmd.Dir = args.baseDir
	bcmd.Stdout = os.Stdout
	bcmd.Stderr = os.Stderr
	if err := bcmd.Run(); err != nil {
		return fmt.Errorf("failed to build command: %w", err)
	}
	defer os.Remove(outpath)

	execCommand := make([]string, 0, len(args.execCommand))
	for _, ec := range args.execCommand {
		if ec == buildOutPathPlaceholder {
			execCommand = append(execCommand, outpath)
		} else {
			execCommand = append(execCommand, ec)
		}
	}

	slog.InfoContext(ctx, "executing command", slog.Any("command", execCommand))
	ecmd := exec.CommandContext(ctx, execCommand[0], execCommand[1:]...)
	ecmd.Dir = args.baseDir
	ecmd.Stdout = os.Stdout
	ecmd.Stderr = os.Stderr
	if args.addr != "" {
		up := udsPath(fname, args.tempDir)
		ecmd.Env = append(ecmd.Env, fmt.Sprintf("%s=%s", defaultTanukiupUDSPathEnv, up))
		waitAndListenProxyServer(ctx, args.addr, args.baseDir, up, args.catchAllTarget)
	}

	if err := ecmd.Run(); err != nil {
		return fmt.Errorf("failed to start command: %w", err)
	}

	return nil
}

type generatorInfo struct {
	command []string
	dir     string
}

func (g *generatorInfo) run(ctx context.Context) error {
	slog.InfoContext(ctx, "running generator", slog.Any("command", g.command), slog.String("dir", g.dir))
	cmd := exec.CommandContext(ctx, g.command[0], g.command[1:]...)
	cmd.Dir = g.dir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to run generator: %w", err)
	}
	return nil
}

func (g *generatorInfo) String() string {
	return fmt.Sprintf("command: %v, dir: %s", g.command, g.dir)
}

var (
	enabledGenerator      = map[string]*generatorInfo{}
	enabledGeneratorMutex = sync.RWMutex{}
)

func runGenerator(ctx context.Context) error {
	enabledGeneratorMutex.RLock()
	defer enabledGeneratorMutex.RUnlock()
	for _, generator := range enabledGenerator {
		if err := generator.run(ctx); err != nil {
			return fmt.Errorf("failed to search generate: %w", err)
		}
	}
	return nil
}

func enableGenerator(ctx context.Context, generator *generatorInfo) {
	enabledGeneratorMutex.Lock()
	defer enabledGeneratorMutex.Unlock()
	enabledGenerator[generator.String()] = generator
	slog.InfoContext(ctx, "detect generator", slog.String("generator", generator.String()))
}

func searchGenerate(ctx context.Context, filename string) error {
	f, err := os.Open(filename)
	if err != nil {
		return fmt.Errorf("failed to open file: %w", err)
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		select {
		case <-ctx.Done():
			return nil
		default:
		}
		line := scanner.Text()
		if strings.HasPrefix(line, "//go:generate go run ") {
			fields := strings.Fields(line)
			if len(fields) < 4 {
				continue
			}
			if _, ok := whitelistGenerate[fields[3]]; !ok {
				continue
			}
			enableGenerator(ctx, &generatorInfo{
				command: fields[1:],
				dir:     filepath.Dir(filename),
			})
		}
	}

	return nil
}

type routePath struct {
	Method string `json:"method"`
	Path   string `json:"path"`
}

var routePathsCommand = []string{"go", "run", "github.com/mackee/tanukirpc/cmd/showpaths"}

func retrievePaths(ctx context.Context, basedir string) ([]routePath, error) {
	buf := &bytes.Buffer{}
	ecmd := exec.CommandContext(ctx, routePathsCommand[0], append(routePathsCommand[1:], basedir)...)
	ecmd.Stdout = buf

	if err := ecmd.Run(); err != nil {
		return nil, fmt.Errorf("failed to run showpaths: %w", err)
	}
	type paths struct {
		Paths []routePath `json:"paths"`
	}
	var ps paths
	if err := json.NewDecoder(buf).Decode(&ps); err != nil {
		return nil, fmt.Errorf("failed to decode json: %w", err)
	}

	return ps.Paths, nil
}

func udsPath(fname string, tempDir string) string {
	bd := filepath.Base(fname)
	return filepath.Join(tempDir, bd+".sock")
}

func proxyServer(addr string, routePaths []routePath, udsPath string, catchAllTarget string) (*http.Server, error) {
	transport := &http.Transport{
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			return net.Dial("unix", udsPath)
		},
	}
	u, err := url.Parse("http://" + addr)
	if err != nil {
		return nil, fmt.Errorf("failed to parse url: %w", err)
	}
	appProxy := httputil.NewSingleHostReverseProxy(u)
	appProxy.Transport = transport

	router := chi.NewRouter()
	for _, rp := range routePaths {
		router.Method(rp.Method, rp.Path, appProxy)
	}

	if catchAllTarget != "" {
		u2, err := url.Parse(catchAllTarget)
		if err != nil {
			return nil, fmt.Errorf("failed to parse url: %w", err)
		}
		catchAll := httputil.NewSingleHostReverseProxy(u2)
		router.NotFound(catchAll.ServeHTTP)
	}

	server := &http.Server{
		Addr:    addr,
		Handler: router,
	}

	return server, nil
}

func tryLaunchProxyServer(ctx context.Context, server *http.Server, udsPath string) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		slog.ErrorContext(ctx, "failed to create watcher", slog.Any("error", err))
		return
	}

	dir := filepath.Dir(udsPath)
	slog.InfoContext(ctx, "watching directory", slog.String("directory", dir))
	if err := watcher.Add(dir + "/"); err != nil {
		defer watcher.Close()
		slog.ErrorContext(ctx, "failed to add directory to watcher", slog.Any("error", err))
		return
	}
	go func() {
		defer watcher.Close()
	OUTER:
		for {
			select {
			case <-ctx.Done():
				return
			case event, ok := <-watcher.Events:
				if !ok {
					slog.InfoContext(ctx, "watcher is closed")
					return
				}
				if event.Name == udsPath {
					break OUTER
				}
			}
		}
		watcher.Close()

		go func() {
			<-ctx.Done()
			sctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if err := server.Shutdown(sctx); err != nil {
				slog.ErrorContext(ctx, "failed to shutdown server", slog.Any("error", err))
			}
		}()
		slog.Info("staring proxy server", slog.String("addr", server.Addr))
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.ErrorContext(ctx, "failed to listen and serve", slog.Any("error", err))
		}
	}()
}

func waitAndListenProxyServer(ctx context.Context, addr string, basedir string, up string, catchAllTarget string) {
	rps, err := retrievePaths(ctx, basedir)
	if err != nil {
		slog.ErrorContext(ctx, "failed to retrieve paths", slog.Any("error", err))
		return
	}
	server, err := proxyServer(addr, rps, up, catchAllTarget)
	if err != nil {
		slog.ErrorContext(ctx, "failed to create proxy server", slog.Any("error", err))
		return
	}
	tryLaunchProxyServer(ctx, server, up)
}

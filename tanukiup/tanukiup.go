package tanukiup

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"math/rand/v2"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"github.com/fsnotify/fsnotify"
)

const (
	defaultPort                 = "9180"
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
	fileExts     []string
	dirs         []string
	ignoreDirs   []string
	buildCommand []string
	execCommand  []string
	port         string
	tempDir      string
	logLevel     slog.Level
}

func newDefaultOptionArgs() *optionArgs {
	tempDir := os.TempDir()
	return &optionArgs{
		fileExts:     defaultFileExts,
		dirs:         defaultDirs,
		ignoreDirs:   defaultIgnoreDirs,
		buildCommand: defaultBuildCommand,
		execCommand:  defaultExecCommand,
		port:         defaultPort,
		tempDir:      tempDir,
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

func WithPort(port string) Option {
	return func(args *optionArgs) {
		args.port = port
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

func Run(ctx context.Context, options ...Option) error {
	args := newDefaultOptionArgs()
	Options(options).apply(args)

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: args.logLevel,
	}))

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
					if err := runGenerator(ctx, logger); err != nil {
						var exitError *exec.ExitError
						if !errors.Is(err, context.Canceled) &&
							!errors.As(err, &exitError) &&
							exitError.ExitCode() != -1 {
							logger.Error("failed to generate command", slog.Any("error", err))
							errChan <- err
						}
						return
					}
					if err := startCmd(cmdCtx, args, logger); err != nil {
						var exitError *exec.ExitError
						if !errors.Is(err, context.Canceled) &&
							!errors.As(err, &exitError) &&
							exitError.ExitCode() != -1 {
							logger.Info("exit error", slog.Any("error", errors.Is(err, context.Canceled)))
							logger.Error("failed to start command", slog.Any("error", err))
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
					logger.InfoContext(ctx, "watcher is closed")
					return
				}
				logger.DebugContext(ctx, "event", slog.Any("event", event))
				if event.Has(fsnotify.Write) || event.Has(fsnotify.Create) {
					if _, ok := extMap[filepath.Ext(event.Name)]; !ok {
						continue
					}
					logger.InfoContext(ctx, "modified file", slog.String("filename", event.Name))
					restartChan <- struct{}{}
				}
			case err, ok := <-watcher.Errors:
				if !ok {
					logger.Info("watcher is closed")
					return
				}
				logger.ErrorContext(ctx, "watcher error", slog.Any("error", err))
			}
		}
	}()

	ignoreDirsMap := make(map[string]struct{})
	for _, dir := range args.ignoreDirs {
		ignoreDirsMap[dir] = struct{}{}
	}

	for _, dir := range args.dirs {
		if noRecursive := strings.TrimSuffix(dir, "..."); noRecursive != dir {
			stat, err := os.Stat(noRecursive)
			if err != nil {
				return fmt.Errorf("failed to get directory info: %w", err)
			}
			if !stat.IsDir() {
				return fmt.Errorf("not a directory: %s", noRecursive)
			}
			if err := filepath.WalkDir(noRecursive, walkDirFunc(ctx, logger, ignoreDirsMap, watcher)); err != nil {
				return fmt.Errorf("failed to walk directory: %w", err)
			}
			continue
		}
		if err := walkDirFunc(ctx, logger, ignoreDirsMap, watcher)(dir, nil, nil); err != nil {
			return fmt.Errorf("failed to walk directory: %w", err)
		}
	}

	<-ctx.Done()
	return nil
}

type isDirer interface {
	IsDir() bool
}

func walkDirFunc(ctx context.Context, logger *slog.Logger, ignoreDirsMap map[string]struct{}, watcher *fsnotify.Watcher) fs.WalkDirFunc {
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
				if err := searchGenerate(ctx, filepath.Join(p, f.Name()), logger); err != nil {
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
		logger.Info("watching directory", slog.String("directory", p))
		return nil
	}
}

func startCmd(ctx context.Context, args *optionArgs, logger *slog.Logger) error {
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
	logger.Info("building command", slog.Any("command", buildCommand))
	bcmd := exec.CommandContext(ctx, buildCommand[0], buildCommand[1:]...)
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

	logger.Info("executing command", slog.Any("command", execCommand))
	ecmd := exec.CommandContext(ctx, execCommand[0], execCommand[1:]...)
	ecmd.Stdout = os.Stdout
	ecmd.Stderr = os.Stderr
	if err := ecmd.Run(); err != nil {
		return fmt.Errorf("failed to start command: %w", err)
	}

	return nil
}

type generatorInfo struct {
	command []string
	dir     string
}

func (g *generatorInfo) run(ctx context.Context, logger *slog.Logger) error {
	logger.Info("running generator", slog.Any("command", g.command), slog.String("dir", g.dir))
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

func runGenerator(ctx context.Context, logger *slog.Logger) error {
	enabledGeneratorMutex.RLock()
	defer enabledGeneratorMutex.RUnlock()
	for _, generator := range enabledGenerator {
		if err := generator.run(ctx, logger); err != nil {
			return fmt.Errorf("failed to search generate: %w", err)
		}
	}
	return nil
}

func enableGenerator(generator *generatorInfo, logger *slog.Logger) {
	enabledGeneratorMutex.Lock()
	defer enabledGeneratorMutex.Unlock()
	enabledGenerator[generator.String()] = generator
	logger.Info("detect generator", slog.String("generator", generator.String()))
}

func searchGenerate(ctx context.Context, filename string, logger *slog.Logger) error {
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
			enableGenerator(&generatorInfo{
				command: fields[1:],
				dir:     filepath.Dir(filename),
			}, logger)
		}
	}

	return nil
}

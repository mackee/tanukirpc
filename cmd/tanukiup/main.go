package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/mackee/tanukirpc/tanukiup"
	"github.com/urfave/cli/v2"
)

func main() {
	app := &cli.App{
		Name:  "tanukiup",
		Usage: "tanukiup is a tool to run your server and watch your files",
		Flags: []cli.Flag{
			&cli.StringSliceFlag{
				Name:  "ext",
				Usage: "file extensions to watch",
			},
			&cli.StringSliceFlag{
				Name:  "dir",
				Usage: "directories to watch",
			},
			&cli.StringSliceFlag{
				Name:  "ignore-dir",
				Usage: "directories to ignore",
			},
			&cli.StringFlag{
				Name:        "build",
				Usage:       "build command. {outpath} represents the output path.",
				DefaultText: "go build -o {outpath} ./",
			},
			&cli.StringFlag{
				Name:        "exec",
				Usage:       "exec command. {outpath} represents the output path.",
				DefaultText: "{outpath}",
			},
			&cli.StringFlag{
				Name:  "port",
				Usage: "port number to run the server",
			},
			&cli.StringFlag{
				Name:  "log-level",
				Usage: "log level (debug, info, warn, error)",
			},
		},
		Action: run,
	}

	if err := app.Run(os.Args); err != nil {
		panic(err)
	}
}

func run(cctx *cli.Context) error {
	opts := tanukiup.Options{}
	if exts := cctx.StringSlice("ext"); len(exts) > 0 {
		opts = append(opts, tanukiup.WithFileExts(exts))
	}
	if dirs := cctx.StringSlice("dir"); len(dirs) > 0 {
		opts = append(opts, tanukiup.WithDirs(dirs))
	}
	if ignoreDirs := cctx.StringSlice("ignore-dir"); len(ignoreDirs) > 0 {
		opts = append(opts, tanukiup.WithIgnoreDirs(ignoreDirs))
	}
	if port := cctx.String("port"); port != "" {
		opts = append(opts, tanukiup.WithPort(port))
	}
	if logLevel := cctx.String("log-level"); logLevel != "" {
		levelMap := map[string]slog.Level{
			"debug": slog.LevelDebug,
			"info":  slog.LevelInfo,
			"warn":  slog.LevelWarn,
			"error": slog.LevelError,
		}
		if lv, ok := levelMap[logLevel]; ok {
			opts = append(opts, tanukiup.WithLogLevel(lv))
		} else {
			return fmt.Errorf("unknown log level: %s", logLevel)
		}
	}
	if build := cctx.String("build"); build != "" {
		bc := strings.Fields(build)
		opts = append(opts, tanukiup.WithBuildCommand(bc))
	}
	if exec := cctx.String("exec"); exec != "" {
		ec := strings.Fields(exec)
		opts = append(opts, tanukiup.WithExecCommand(ec))
	}

	ctx, cancel := context.WithCancel(cctx.Context)

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGHUP, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)
	go func() {
		<-sig
		cancel()
	}()

	if err := tanukiup.Run(ctx, opts...); err != nil {
		return fmt.Errorf("failed to run tanukiup: %w", err)
	}
	return nil
}

//go:build unix

package tanukiup

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

const tanukiupHelperEnv = "TANUKIUP_TEST_HELPER_PROCESS"

func TestRunRestartTerminatesDescendantProcesses(t *testing.T) {
	t.Setenv(tanukiupHelperEnv, "1")

	tempDir := t.TempDir()
	watchDir := filepath.Join(tempDir, "watch")
	if err := os.Mkdir(watchDir, 0o755); err != nil {
		t.Fatal(err)
	}
	watchFile := filepath.Join(watchDir, "main.go")
	if err := os.WriteFile(watchFile, []byte("package main\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	pidFile := filepath.Join(tempDir, "grandchild.pid")
	readyFile := filepath.Join(tempDir, "ready")
	helperCommand := []string{
		os.Args[0],
		"-test.run=^TestTanukiupHelperProcess$",
		"--",
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- Run(ctx,
			WithDirs([]string{watchDir}),
			WithFileExts([]string{".go"}),
			WithBuildCommand(append(slices.Clone(helperCommand), "build")),
			WithExecCommand(append(slices.Clone(helperCommand), "parent", pidFile, readyFile)),
			WithLogLevel(slog.LevelError),
			WithTempDir(tempDir),
		)
	}()
	defer func() {
		cancel()
		select {
		case err := <-done:
			if err != nil {
				t.Errorf("tanukiup returned error: %v", err)
			}
		case <-time.After(5 * time.Second):
			t.Errorf("tanukiup did not stop")
		}
	}()

	waitForFile(t, readyFile, 5*time.Second)
	oldPID := readPIDFile(t, pidFile)
	cleanupOldPID := true
	defer func() {
		if cleanupOldPID {
			killPID(oldPID)
		}
	}()
	if !pidExists(oldPID) {
		t.Fatalf("grandchild process did not start: pid=%d", oldPID)
	}

	if err := os.Remove(readyFile); err != nil {
		t.Fatal(err)
	}
	f, err := os.OpenFile(watchFile, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString("\n"); err != nil {
		_ = f.Close()
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}

	waitForFile(t, readyFile, 5*time.Second)
	newPID := readPIDFile(t, pidFile)
	defer killPID(newPID)
	if newPID == oldPID {
		t.Fatalf("restart did not replace grandchild process: pid=%d", oldPID)
	}

	if stillRunning := waitForPIDExit(oldPID, 2*time.Second); stillRunning {
		t.Fatalf("old descendant process is still running after restart: pid=%d", oldPID)
	}
	cleanupOldPID = false
}

func TestRunCancelWaitsForDescendantCleanup(t *testing.T) {
	t.Setenv(tanukiupHelperEnv, "1")

	tempDir := t.TempDir()
	watchDir := filepath.Join(tempDir, "watch")
	if err := os.Mkdir(watchDir, 0o755); err != nil {
		t.Fatal(err)
	}

	pidFile := filepath.Join(tempDir, "grandchild.pid")
	readyFile := filepath.Join(tempDir, "ready")
	helperCommand := []string{
		os.Args[0],
		"-test.run=^TestTanukiupHelperProcess$",
		"--",
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- Run(ctx,
			WithDirs([]string{watchDir}),
			WithBuildCommand(append(slices.Clone(helperCommand), "build")),
			WithExecCommand(append(slices.Clone(helperCommand), "parent", pidFile, readyFile)),
			WithLogLevel(slog.LevelError),
			WithTempDir(tempDir),
		)
	}()

	waitForFile(t, readyFile, 5*time.Second)
	pid := readPIDFile(t, pidFile)
	cleanupPID := true
	defer func() {
		if cleanupPID {
			killPID(pid)
		}
	}()
	if !pidExists(pid) {
		t.Fatalf("grandchild process did not start: pid=%d", pid)
	}

	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("tanukiup returned error: %v", err)
		}
	case <-time.After(6 * time.Second):
		t.Fatal("tanukiup did not stop")
	}
	if pidExists(pid) {
		t.Fatalf("descendant process is still running after Run returned: pid=%d", pid)
	}
	cleanupPID = false
}

func TestRunCommandGivesDescendantsShutdownGracePeriod(t *testing.T) {
	t.Setenv(tanukiupHelperEnv, "1")

	tempDir := t.TempDir()
	terminatedFile := filepath.Join(tempDir, "terminated")
	exitedFile := filepath.Join(tempDir, "exited")
	ctx, cancel := context.WithCancel(context.Background())
	cmd := newCommand(
		os.Args[0],
		"-test.run=^TestTanukiupHelperProcess$",
		"--",
		"parent-graceful-grandchild",
		terminatedFile,
		exitedFile,
	)
	cmd.Env = os.Environ()
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard

	done := make(chan error, 1)
	go func() {
		done <- runCommand(ctx, cmd)
	}()

	waitForFile(t, terminatedFile, 5*time.Second)
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("runCommand returned error: %v", err)
		}
	case <-time.After(6 * time.Second):
		t.Fatal("runCommand did not stop")
	}
	if _, err := os.Stat(exitedFile); err != nil {
		t.Fatalf("grandchild did not exit gracefully: %v", err)
	}
}

func TestTanukiupHelperProcess(t *testing.T) {
	if os.Getenv(tanukiupHelperEnv) != "1" {
		return
	}
	args := os.Args
	idx := slices.Index(args, "--")
	if idx == -1 || idx+1 >= len(args) {
		fmt.Fprintln(os.Stderr, "missing helper mode")
		os.Exit(2)
	}

	switch mode := args[idx+1]; mode {
	case "build":
		os.Exit(0)
	case "parent":
		if idx+3 >= len(args) {
			fmt.Fprintln(os.Stderr, "missing parent helper args")
			os.Exit(2)
		}
		runTanukiupParentHelper(args[idx+2], args[idx+3])
	case "parent-graceful-grandchild":
		if idx+3 >= len(args) {
			fmt.Fprintln(os.Stderr, "missing parent-graceful-grandchild helper args")
			os.Exit(2)
		}
		runTanukiupParentGracefulGrandchildHelper(args[idx+2], args[idx+3])
	case "graceful-grandchild":
		if idx+3 >= len(args) {
			fmt.Fprintln(os.Stderr, "missing graceful-grandchild helper args")
			os.Exit(2)
		}
		runTanukiupGracefulGrandchildHelper(args[idx+2], args[idx+3])
	case "grandchild":
		for {
			time.Sleep(time.Hour)
		}
	default:
		fmt.Fprintf(os.Stderr, "unknown helper mode: %s\n", mode)
		os.Exit(2)
	}
}

func runTanukiupParentHelper(pidFile, readyFile string) {
	cmd := exec.Command(os.Args[0], "-test.run=^TestTanukiupHelperProcess$", "--", "grandchild")
	cmd.Env = os.Environ()
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		fmt.Fprintf(os.Stderr, "failed to start grandchild: %v\n", err)
		os.Exit(2)
	}
	go func() {
		_ = cmd.Wait()
	}()
	if err := os.WriteFile(pidFile, []byte(strconv.Itoa(cmd.Process.Pid)), 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "failed to write pid file: %v\n", err)
		os.Exit(2)
	}
	if err := os.WriteFile(readyFile, []byte("ready"), 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "failed to write ready file: %v\n", err)
		os.Exit(2)
	}
	for {
		time.Sleep(time.Hour)
	}
}

func runTanukiupParentGracefulGrandchildHelper(terminatedFile, exitedFile string) {
	cmd := exec.Command(os.Args[0], "-test.run=^TestTanukiupHelperProcess$", "--", "graceful-grandchild", terminatedFile, exitedFile)
	cmd.Env = os.Environ()
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		fmt.Fprintf(os.Stderr, "failed to start graceful grandchild: %v\n", err)
		os.Exit(2)
	}
	go func() {
		_ = cmd.Wait()
	}()
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGTERM)
	defer signal.Stop(sig)
	<-sig
	os.Exit(0)
}

func runTanukiupGracefulGrandchildHelper(terminatedFile, exitedFile string) {
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGTERM)
	defer signal.Stop(sig)
	if err := os.WriteFile(terminatedFile, []byte("ready"), 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "failed to write terminated file: %v\n", err)
		os.Exit(2)
	}
	<-sig
	time.Sleep(200 * time.Millisecond)
	if err := os.WriteFile(exitedFile, []byte("exited"), 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "failed to write exited file: %v\n", err)
		os.Exit(2)
	}
	os.Exit(0)
}

func waitForFile(t *testing.T, path string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(path); err == nil {
			return
		} else if !errors.Is(err, os.ErrNotExist) {
			t.Fatal(err)
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", path)
}

func readPIDFile(t *testing.T, path string) int {
	t.Helper()
	bs, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(bs)))
	if err != nil {
		t.Fatal(err)
	}
	return pid
}

func waitForPIDExit(pid int, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if !pidExists(pid) {
			return false
		}
		time.Sleep(20 * time.Millisecond)
	}
	return pidExists(pid)
}

func pidExists(pid int) bool {
	if pid <= 0 {
		return false
	}
	err := syscall.Kill(pid, 0)
	return err == nil || errors.Is(err, syscall.EPERM)
}

func killPID(pid int) {
	if pid <= 0 {
		return
	}
	_ = syscall.Kill(pid, syscall.SIGKILL)
}

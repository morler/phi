package lisp

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/glojurelang/glojure/pkg/lang"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWorkerProcessGroup(t *testing.T) {
	switch os.Getenv("PHI_PROCESS_PROBE") {
	case "supervisor":
		processSupervisor(t)
	case "worker":
		processWorker(t)
	case "same", "escaped":
		data, err := io.ReadAll(os.Stdin)
		require.NoError(t, err)
		if string(data) == "go" {
			require.NoError(t, os.WriteFile(os.Getenv("PHI_PROCESS_PROBE")+".txt", data, 0o600))
		}
	default:
		ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestWorkerProcessGroup$")
		cmd.Dir = t.TempDir()
		cmd.Env = []string{"PHI_PROCESS_PROBE=supervisor", "HOME=" + cmd.Dir, "TMPDIR=" + cmd.Dir}
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, "%s", out)
	}
}

func processSupervisor(t *testing.T) {
	t.Helper()
	// Keep orphaned test descendants here so the probe can reap them itself.
	const prSetChildSubreaper = 36
	_, _, errno := syscall.RawSyscall6(syscall.SYS_PRCTL, prSetChildSubreaper, 1, 0, 0, 0, 0)
	require.Zero(t, errno)
	reader, writer, err := os.Pipe()
	require.NoError(t, err)
	defer writer.Close()
	ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestWorkerProcessGroup$")
	cmd.Env = append(os.Environ(), "PHI_PROCESS_PROBE=worker")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true, Pdeathsig: syscall.SIGKILL}
	cmd.Stdin = reader
	stdout, err := cmd.StdoutPipe()
	require.NoError(t, err)
	cmd.Stderr = os.Stderr
	require.NoError(t, cmd.Start())
	require.NoError(t, reader.Close())
	defer func() {
		_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		_ = writer.Close()
		if cmd.ProcessState == nil {
			_ = cmd.Wait()
		}
		for {
			var status syscall.WaitStatus
			if _, waitErr := syscall.Wait4(-1, &status, 0, nil); waitErr != nil {
				break
			}
		}
	}()
	lines := make(chan string, 3)
	go func() {
		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			lines <- scanner.Text()
		}
		close(lines)
	}()
	pids := make(map[string]int)
	for len(pids) < 2 {
		select {
		case line, ok := <-lines:
			require.True(t, ok, "worker exited before starting both descendants")
			fields := strings.Fields(line)
			require.Len(t, fields, 2)
			pid, parseErr := strconv.Atoi(fields[1])
			require.NoError(t, parseErr)
			pids[fields[0]] = pid
		case <-ctx.Done():
			require.FailNow(t, "worker did not start descendants", "%v", ctx.Err())
		}
	}
	require.Positive(t, pids["same"])
	require.Positive(t, pids["escaped"])
	select {
	case line, ok := <-lines:
		require.True(t, ok)
		require.Equal(t, "eval-ready", line)
	case <-ctx.Done():
		require.FailNow(t, "evaluation did not start", "%v", ctx.Err())
	}
	require.NoError(t, syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL))
	require.Error(t, cmd.Wait())
	var status syscall.WaitStatus
	pid, err := syscall.Wait4(pids["same"], &status, 0, nil)
	require.NoError(t, err)
	assert.Equal(t, pids["same"], pid)
	assert.True(t, status.Signaled())
	assert.Equal(t, syscall.SIGKILL, status.Signal())
	pid, err = syscall.Wait4(pids["escaped"], &status, syscall.WNOHANG, nil)
	require.NoError(t, err)
	assert.Zero(t, pid, "escaped child unexpectedly exited")
	_, err = writer.WriteString("go")
	require.NoError(t, err)
	require.NoError(t, writer.Close())
	pid, err = syscall.Wait4(pids["escaped"], &status, 0, nil)
	require.NoError(t, err)
	assert.Equal(t, pids["escaped"], pid)
	assert.True(t, status.Exited())
	assert.Zero(t, status.ExitStatus())
	data, err := os.ReadFile(filepath.Join(".", "escaped.txt"))
	require.NoError(t, err)
	assert.Equal(t, "go", string(data))
	_, err = os.Stat("same.txt")
	assert.ErrorIs(t, err, os.ErrNotExist)
}

func processWorker(t *testing.T) {
	t.Helper()
	for _, mode := range []string{"same", "escaped"} {
		cmd := exec.CommandContext(t.Context(), os.Args[0], "-test.run=^TestWorkerProcessGroup$")
		cmd.Env = append(os.Environ(), "PHI_PROCESS_PROBE="+mode)
		cmd.Stdin, cmd.Stderr = os.Stdin, os.Stderr
		if mode == "escaped" {
			cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
		}
		require.NoError(t, cmd.Start())
		_, err := fmt.Fprintln(os.Stdout, mode, cmd.Process.Pid)
		require.NoError(t, err)
	}
	rt, err := New()
	require.NoError(t, err)
	lang.FindOrCreateNamespace(lang.NewSymbol("user")).InternWithValue(
		lang.NewSymbol("phi-process-wait"), lang.FnFunc0(func() any {
			_, _ = fmt.Fprintln(os.Stdout, "eval-ready")
			select {}
		}), true,
	)
	_, err = rt.Eval(t.Context(), `(phi-process-wait)`)
	require.NoError(t, err)
}

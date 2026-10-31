package lisp

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/glojurelang/glojure/pkg/lang"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRuntimeRecovery(t *testing.T) {
	mode := os.Getenv("PHI_RECOVERY_PROBE")
	if mode != "" {
		recoveryProbe(t, mode)
		return
	}

	dir := t.TempDir()
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	command := func(mode string) *exec.Cmd {
		cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestRuntimeRecovery$")
		cmd.Dir = dir
		cmd.Env = []string{"PHI_RECOVERY_PROBE=" + mode, "HOME=" + dir, "TMPDIR=" + dir}
		return cmd
	}

	out, err := command("cancel").CombinedOutput()
	require.NoError(t, err, "%s", out)
	content, err := os.ReadFile(filepath.Join(dir, "after-cancel.txt"))
	require.NoError(t, err)
	assert.Equal(t, "ran", string(content))

	cmd := command("hold")
	stdout, err := cmd.StdoutPipe()
	require.NoError(t, err)
	cmd.Stderr = os.Stderr
	require.NoError(t, cmd.Start())
	t.Cleanup(func() {
		if cmd.ProcessState == nil {
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
		}
	})
	ready := make(chan bool, 1)
	go func() {
		scanner := bufio.NewScanner(stdout)
		ready <- scanner.Scan() && scanner.Text() == "ready"
	}()
	select {
	case ok := <-ready:
		require.True(t, ok, "worker did not reach the blocking evaluation")
	case <-ctx.Done():
		require.FailNow(t, "worker did not become ready", "%v", ctx.Err())
	}
	require.NoError(t, cmd.Process.Kill())
	require.Error(t, cmd.Wait())
	require.NoError(t, ctx.Err())
	content, err = os.ReadFile(filepath.Join(dir, "effect.txt"))
	require.NoError(t, err)
	assert.Equal(t, "committed", string(content))

	out, err = command("restart").CombinedOutput()
	require.NoError(t, err, "%s", out)
}

func recoveryProbe(t *testing.T, mode string) {
	t.Helper()
	rt, err := New()
	require.NoError(t, err)
	if mode == "restart" {
		result, evalErr := rt.Eval(t.Context(), `(resolve 'phi-recovery-definition)`)
		require.NoError(t, evalErr)
		assert.Equal(t, "nil", result.Value)
		return
	}

	started, release, finished := make(chan struct{}), make(chan struct{}), make(chan error, 1)
	lang.FindOrCreateNamespace(lang.NewSymbol("user")).InternWithValue(
		lang.NewSymbol("phi-recovery-wait"), lang.FnFunc0(func() any {
			close(started)
			<-release
			finished <- os.WriteFile("after-cancel.txt", []byte("ran"), 0o600)
			return nil
		}), true,
	)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	var releaseOnce sync.Once
	unblock := func() { releaseOnce.Do(func() { close(release) }) }
	defer unblock()
	code := `(phi-recovery-wait)`
	if mode == "hold" {
		code = `(def phi-recovery-definition 42) (os.WriteFile "effect.txt" (.getBytes "committed") 384) (phi-recovery-wait)`
	}
	result := make(chan error, 1)
	go func() {
		_, evalErr := rt.Eval(ctx, code)
		result <- evalErr
	}()
	select {
	case <-started:
	case <-time.After(10 * time.Second):
		require.FailNow(t, "evaluation did not start")
	}
	if mode == "hold" {
		_, err = fmt.Fprintln(os.Stdout, "ready")
		require.NoError(t, err)
		<-t.Context().Done()
		return
	}
	cancel()
	select {
	case err = <-result:
		require.ErrorIs(t, err, context.Canceled)
		assert.Contains(t, err.Error(), "may still be running")
	case <-time.After(10 * time.Second):
		require.FailNow(t, "cancelled evaluation did not return")
	}
	_, err = rt.Eval(t.Context(), `42`)
	require.ErrorIs(t, err, ErrUnavailable)
	unblock()
	select {
	case err = <-finished:
		require.NoError(t, err)
	case <-time.After(10 * time.Second):
		require.FailNow(t, "evaluation did not continue after cancellation")
	}
}

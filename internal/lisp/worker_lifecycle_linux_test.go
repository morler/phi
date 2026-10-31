package lisp

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/glojurelang/glojure/pkg/lang"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIsolatedRuntimeLifecycle(t *testing.T) {
	if os.Getenv("PHI_LIFECYCLE_PROBE") == "worker" {
		rt, err := New()
		require.NoError(t, err)
		lang.FindOrCreateNamespace(lang.NewSymbol("user")).InternWithValue(
			lang.NewSymbol("phi-lifecycle-wait"), lang.FnFunc0(func() any {
				_, _ = fmt.Fprintln(os.Stdout, `"running"`)
				<-time.After(time.Minute)
				return nil
			}), true,
		)
		decoder := json.NewDecoder(os.Stdin)
		for {
			var code string
			if err = decoder.Decode(&code); err != nil {
				return
			}
			result, evalErr := rt.Eval(t.Context(), code)
			require.NoError(t, evalErr)
			require.NoError(t, json.NewEncoder(os.Stdout).Encode(result.Value))
		}
	}
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestIsolatedRuntimeLifecycle$")
	cmd.Dir = t.TempDir()
	cmd.Env = []string{"PHI_LIFECYCLE_PROBE=worker", "HOME=" + cmd.Dir, "TMPDIR=" + cmd.Dir}
	cmd.SysProcAttr = namespaceAttributes()
	input, err := cmd.StdinPipe()
	require.NoError(t, err)
	output, err := cmd.StdoutPipe()
	require.NoError(t, err)
	cmd.Stderr = os.Stderr
	if err = cmd.Start(); err != nil {
		if errors.Is(err, syscall.EPERM) || errors.Is(err, syscall.EACCES) || errors.Is(err, syscall.ENOSYS) {
			t.Skipf("PID/user namespaces unavailable: %v", err)
		}
		require.NoError(t, err)
	}
	t.Cleanup(func() {
		if cmd.ProcessState == nil {
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
		}
	})
	responses := make(chan string, 4)
	go func() {
		scanner := bufio.NewScanner(output)
		for scanner.Scan() {
			responses <- scanner.Text()
		}
		close(responses)
	}()
	send := func(code, expected string) {
		t.Helper()
		require.NoError(t, json.NewEncoder(input).Encode(code))
		select {
		case line, ok := <-responses:
			require.True(t, ok, "worker exited before responding")
			var value string
			require.NoError(t, json.Unmarshal([]byte(line), &value))
			assert.Equal(t, expected, value)
		case <-ctx.Done():
			require.FailNow(t, "worker did not respond", "%v", ctx.Err())
		}
	}
	send(`(def phi-lifecycle-value 41)`, "#'user/phi-lifecycle-value")
	send(`(+ phi-lifecycle-value 1)`, "42")
	send(`(phi-lifecycle-wait)`, "running")
	require.NoError(t, cmd.Process.Kill())
	require.Error(t, cmd.Wait())
	require.NoError(t, ctx.Err())
	select {
	case _, ok := <-responses:
		assert.False(t, ok, "worker emitted data after termination")
	case <-ctx.Done():
		require.FailNow(t, "worker output did not close", "%v", ctx.Err())
	}
	restarted := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestIsolatedRuntimeLifecycle$")
	restarted.Dir, restarted.Env, restarted.SysProcAttr = cmd.Dir, cmd.Env, namespaceAttributes()
	restarted.Stdin = strings.NewReader(`"(resolve 'phi-lifecycle-value)"` + "\n")
	out, err := restarted.CombinedOutput()
	require.NoError(t, err, "%s", out)
	assert.Contains(t, string(out), `"nil"`)
}

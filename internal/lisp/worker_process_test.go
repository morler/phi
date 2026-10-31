package lisp

import (
	"context"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWorkerCloseDuringEvaluation(t *testing.T) {
	if os.Getenv("PHI_WORKER_TEST") == "close" {
		data := make([]byte, 128)
		_, err := os.Stdin.Read(data)
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(os.Getenv("PHI_READY_FILE"), []byte("ready"), 0o600))
		<-time.After(time.Minute)
		return
	}
	ready := t.TempDir() + "/ready"
	cmd := exec.CommandContext(t.Context(), os.Args[0], "-test.run=^TestWorkerCloseDuringEvaluation$")
	cmd.Env = append(os.Environ(), "PHI_WORKER_TEST=close", "PHI_READY_FILE="+ready)
	w, err := StartWorker(cmd)
	require.NoError(t, err)
	t.Cleanup(func() { _ = w.Close() })
	done := make(chan error, 1)
	go func() {
		_, evalErr := w.Eval(t.Context(), strings.Repeat("x", 10))
		done <- evalErr
	}()
	require.Eventually(
		t,
		func() bool { _, statErr := os.Stat(ready); return statErr == nil },
		10*time.Second,
		time.Millisecond,
	)
	closed := make(chan error, 1)
	go func() { closed <- w.Close() }()
	select {
	case err = <-closed:
		require.NoError(t, err)
	case <-time.After(10 * time.Second):
		require.FailNow(t, "close did not stop the active evaluation")
	}
	select {
	case err = <-done:
		require.Error(t, err)
	case <-time.After(10 * time.Second):
		require.FailNow(t, "evaluation did not return after close")
	}
	assert.NotNil(t, cmd.ProcessState)
}

func TestWorkerProcess(t *testing.T) {
	if os.Getenv("PHI_WORKER_TEST") == "1" {
		require.NoError(t, ServeWorker(os.Stdin, os.Stdout))
		return
	}
	cmd := exec.CommandContext(t.Context(), os.Args[0], "-test.run=^TestWorkerProcess$")
	cmd.Env = append(os.Environ(), "PHI_WORKER_TEST=1")
	w, err := StartWorker(cmd)
	require.NoError(t, err)
	t.Cleanup(func() { _ = w.Close() })
	_, err = w.Eval(t.Context(), `(def phi-process-value 41)`)
	require.NoError(t, err)
	result, err := w.Eval(t.Context(), `(+ phi-process-value 1)`)
	require.NoError(t, err)
	assert.Equal(t, "42", result.Value)
	_, err = w.Eval(t.Context(), `(throw (Exception. "boom"))`)
	require.ErrorContains(t, err, "boom")
	result, err = w.Eval(t.Context(), `42`)
	require.NoError(t, err)
	assert.Equal(t, "42", result.Value)
	ctx, cancel := context.WithTimeout(t.Context(), 200*time.Millisecond)
	defer cancel()
	_, err = w.Eval(ctx, `@(promise)`)
	require.ErrorIs(t, err, context.DeadlineExceeded)
	assert.NotNil(t, cmd.ProcessState)
	_, err = w.Eval(t.Context(), `42`)
	require.ErrorIs(t, err, ErrUnavailable)
}

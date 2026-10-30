package lisp

import (
	"context"
	"os"
	"os/exec"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRuntime(t *testing.T) {
	rt, err := New()
	require.NoError(t, err)

	other, err := New()
	require.NoError(t, err)
	assert.Same(t, rt, other)

	result, err := rt.Eval(t.Context(), `(defn phi-runtime-double [n] (* n 2))`)
	require.NoError(t, err)
	assert.NotEmpty(t, result.Value)

	result, err = other.Eval(t.Context(), `(println "hello") (phi-runtime-double 21)`)
	require.NoError(t, err)
	assert.Equal(t, "42", result.Value)
	assert.Equal(t, "hello\n", result.Stdout)
	assert.Empty(t, result.Stderr)

	result, err = rt.Eval(t.Context(), `(binding [*out* *err*] (println "warning")) (throw (Exception. "boom"))`)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "boom")
	assert.Equal(t, "warning\n", result.Stderr)

	_, err = rt.Eval(t.Context(), `(broken`)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "read Glojure")

	result, err = rt.Eval(t.Context(), `(phi-runtime-double 2)`)
	require.NoError(t, err)
	assert.Equal(t, "4", result.Value)

	_, err = rt.Eval(t.Context(), `(ns phi.runtime-test) (def saved-value 17)`)
	require.NoError(t, err)
	result, err = rt.Eval(t.Context(), `saved-value`)
	require.NoError(t, err)
	assert.Equal(t, "17", result.Value)
	_, err = rt.Eval(t.Context(), `(ns user)`)
	require.NoError(t, err)
}

func TestRuntimeSerializesConcurrentCalls(t *testing.T) {
	rt, err := New()
	require.NoError(t, err)
	_, err = rt.Eval(t.Context(), `(def phi-runtime-counter (atom 0))`)
	require.NoError(t, err)

	var wg sync.WaitGroup
	for range 10 {
		wg.Go(func() {
			_, err := rt.Eval(t.Context(), `(swap! phi-runtime-counter inc)`)
			assert.NoError(t, err)
		})
	}
	wg.Wait()
	result, err := rt.Eval(t.Context(), `@phi-runtime-counter`)
	require.NoError(t, err)
	assert.Equal(t, "10", result.Value)
}

func TestRuntimeCancellation(t *testing.T) {
	// The abandoned evaluation cannot be interrupted, so isolate it from other tests.
	if os.Getenv("PHI_LISP_CANCEL_TEST") == "1" {
		rt, err := New()
		require.NoError(t, err)

		ctx, cancel := context.WithCancel(t.Context())
		cancel()
		_, err = rt.Eval(ctx, `(def should-not-run true)`)
		require.ErrorIs(t, err, context.Canceled)
		result, err := rt.Eval(t.Context(), `(resolve 'should-not-run)`)
		require.NoError(t, err)
		assert.Equal(t, "nil", result.Value)

		ctx, cancel = context.WithTimeout(t.Context(), 200*time.Millisecond)
		defer cancel()
		_, err = rt.Eval(ctx, `@(promise)`)
		require.ErrorIs(t, err, context.DeadlineExceeded)
		assert.Contains(t, err.Error(), "may still be running")

		_, err = rt.Eval(t.Context(), `42`)
		require.ErrorIs(t, err, ErrUnavailable)
		return
	}

	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestRuntimeCancellation$")
	cmd.Env = append(os.Environ(), "PHI_LISP_CANCEL_TEST=1")
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "%s", out)
}

func TestRuntimeStopped(t *testing.T) {
	rt := &Runtime{requests: make(chan request), done: make(chan struct{})}
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Millisecond)
	defer cancel()
	_, err := rt.Eval(ctx, `42`)
	require.ErrorIs(t, err, context.DeadlineExceeded)
	select {
	case <-rt.done:
		assert.Fail(t, "a cancelled queued request stopped the runtime")
	default:
	}

	rt.stop()
	_, err = rt.Eval(t.Context(), `42`)
	require.ErrorIs(t, err, ErrUnavailable)
}

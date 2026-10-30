// Package lisp owns the process-wide Glojure evaluation loop.
package lisp

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"

	_ "github.com/glojurelang/glojure/pkg/glj" // Register the Glojure standard library.
	"github.com/glojurelang/glojure/pkg/lang"
	"github.com/glojurelang/glojure/pkg/reader"
	"github.com/glojurelang/glojure/pkg/runtime"
)

// Runtime serializes evaluations in one Glojure environment for the process lifetime.
type Runtime struct {
	requests chan request
	done     chan struct{}
	stopOnce sync.Once
}

// Result contains the last value and output produced by an evaluation.
type Result struct {
	Value  string
	Stdout string
	Stderr string
}

type request struct {
	code  string
	reply chan response
}

type response struct {
	result Result
	err    error
}

var (
	// ErrUnavailable means the evaluation loop has stopped or was abandoned.
	ErrUnavailable = errors.New("glojure runtime is unavailable")

	once     sync.Once
	instance *Runtime
	startErr error
)

// New returns the shared runtime. Glojure's environment and namespaces are global.
func New() (*Runtime, error) {
	once.Do(func() {
		instance = &Runtime{requests: make(chan request), done: make(chan struct{})}
		ready := make(chan error, 1)
		go instance.run(ready)
		startErr = <-ready
		if startErr != nil {
			instance = nil
		}
	})
	return instance, startErr
}

// Eval evaluates all forms in code and returns the last value.
// Definitions and the current namespace remain available to later calls.
// Cancellation after execution starts makes the runtime unavailable: Glojure
// cannot interrupt the evaluation, which may continue to have side effects.
func (r *Runtime) Eval(ctx context.Context, code string) (Result, error) {
	if err := ctx.Err(); err != nil {
		return Result{}, err
	}
	reply := make(chan response)
	select {
	case <-ctx.Done():
		return Result{}, ctx.Err()
	case <-r.done:
		return Result{}, ErrUnavailable
	case r.requests <- request{code: code, reply: reply}:
	}
	select {
	case <-ctx.Done():
		// shortcut: a running form keeps executing; use process isolation to stop it.
		r.stop()
		return Result{}, fmt.Errorf("glojure evaluation may still be running: %w", ctx.Err())
	case <-r.done:
		return Result{}, ErrUnavailable
	case out := <-reply:
		return out.result, out.err
	}
}

func (r *Runtime) stop() {
	r.stopOnce.Do(func() { close(r.done) })
}

func (r *Runtime) run(ready chan<- error) {
	defer r.stop()
	env, err := initialize()
	ready <- err
	if err != nil {
		return
	}
	defer lang.PopThreadBindings()
	for {
		select {
		case <-r.done:
			return
		case req := <-r.requests:
			select {
			case <-r.done:
				return
			default:
			}
			result, err := evaluate(env, req.code, r.done)
			select {
			case req.reply <- response{result: result, err: err}:
			case <-r.done:
				return
			}
		}
	}
}

func initialize() (env lang.Environment, err error) {
	bound := false
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("initialize Glojure: %v", recovered)
		}
		if err != nil && bound {
			lang.PopThreadBindings()
		}
	}()
	bindings := lang.NewMap(
		lang.VarCurrentNS, lang.VarCurrentNS.Deref(),
		lang.VarWarnOnReflection, lang.VarWarnOnReflection.Deref(),
		lang.VarUncheckedMath, lang.VarUncheckedMath.Deref(),
		lang.VarDataReaders, lang.VarDataReaders.Deref(),
	)
	lang.PushThreadBindings(bindings)
	bound = true
	env = runtime.NewEnvironment(runtime.WithStdout(io.Discard), runtime.WithStderr(io.Discard))
	if _, err = env.Eval(lang.NewList(lang.NewSymbol("ns"), lang.NewSymbol("user"))); err != nil {
		return nil, fmt.Errorf("initialize Glojure namespace: %w", err)
	}
	return env, nil
}

func evaluate(env lang.Environment, code string, done <-chan struct{}) (result Result, err error) {
	stdout, stderr := &lockedBuffer{}, &lockedBuffer{}
	defer func() {
		result.Stdout, result.Stderr = stdout.String(), stderr.String()
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("evaluate Glojure: %v", recovered)
		}
	}()
	errVar := lang.NSCore.FindInternedVar(lang.NewSymbol("*err*"))
	lang.PushThreadBindings(lang.NewMap(lang.VarOut, stdout, errVar, stderr))
	defer lang.PopThreadBindings()

	r := reader.New(strings.NewReader(code), reader.WithGetCurrentNS(env.CurrentNamespace))
	var value any
	for {
		select {
		case <-done:
			return result, ErrUnavailable
		default:
		}
		form, readErr := r.ReadOne()
		if errors.Is(readErr, reader.ErrEOF) {
			break
		}
		if readErr != nil {
			return result, fmt.Errorf("read Glojure: %w", readErr)
		}
		value, err = env.Eval(form)
		if err != nil {
			return result, fmt.Errorf("evaluate Glojure: %w", err)
		}
	}
	select {
	case <-done:
		return result, ErrUnavailable
	default:
	}
	result.Value = lang.PrintString(value)
	return result, nil
}

type lockedBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (b *lockedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.b.Write(p)
}

func (b *lockedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.b.String()
}

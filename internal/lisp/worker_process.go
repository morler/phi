package lisp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sync"
)

// WorkerProcess serializes requests to a caller-supplied isolated subprocess.
// The caller owns isolation and must not pass an unrestricted command.
type WorkerProcess struct {
	cmd      *exec.Cmd
	input    io.WriteCloser
	output   io.ReadCloser
	lock     chan struct{}
	stopOnce sync.Once
	stopped  chan struct{}
	stopErr  error
}

// StartWorker starts an already configured isolated subprocess.
func StartWorker(cmd *exec.Cmd) (*WorkerProcess, error) {
	in, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("open worker input: %w", err)
	}
	out, err := cmd.StdoutPipe()
	if err != nil {
		_ = in.Close()
		return nil, fmt.Errorf("open worker output: %w", err)
	}
	if err := cmd.Start(); err != nil {
		_ = in.Close()
		_ = out.Close()
		return nil, fmt.Errorf("start worker: %w", err)
	}
	w := &WorkerProcess{cmd: cmd, input: in, output: out, lock: make(chan struct{}, 1), stopped: make(chan struct{})}
	w.lock <- struct{}{}
	return w, nil
}

// Eval terminates and reaps the worker if the request is canceled in flight.
func (w *WorkerProcess) Eval(ctx context.Context, code string) (Result, error) {
	select {
	case <-ctx.Done():
		return Result{}, ctx.Err()
	case <-w.stopped:
		return Result{}, ErrUnavailable
	case <-w.lock:
	}
	defer func() { w.lock <- struct{}{} }()
	select {
	case <-w.stopped:
		return Result{}, ErrUnavailable
	default:
	}
	if err := ctx.Err(); err != nil {
		return Result{}, err
	}
	type outcome struct {
		response
		protocolError bool
	}
	responses := make(chan outcome, 1)
	go func() {
		if err := json.NewEncoder(w.input).Encode(code); err != nil {
			responses <- outcome{response: response{err: err}, protocolError: true}
			return
		}
		var reply workerReply
		if err := json.NewDecoder(w.output).Decode(&reply); err != nil {
			responses <- outcome{response: response{err: err}, protocolError: true}
			return
		}
		var err error
		if reply.Error != "" {
			err = errors.New(reply.Error)
		}
		responses <- outcome{response: response{result: reply.Result, err: err}}
	}()
	select {
	case <-ctx.Done():
		stopErr := w.stop()
		if stopErr != nil {
			return Result{}, errors.Join(ctx.Err(), stopErr)
		}
		<-responses
		return Result{}, ctx.Err()
	case <-w.stopped:
		if w.stopErr == nil {
			<-responses
		}
		return Result{}, ErrUnavailable
	case out := <-responses:
		if out.protocolError {
			// A protocol failure cannot leave a worker reusable.
			_ = w.stop()
		}
		return out.result, out.err
	}
}

// Close terminates and reaps the worker.
func (w *WorkerProcess) Close() error {
	return w.stop()
}

func (w *WorkerProcess) stop() error {
	w.stopOnce.Do(func() {
		err := w.cmd.Process.Kill()
		if err != nil && !errors.Is(err, os.ErrProcessDone) {
			w.stopErr = fmt.Errorf("stop worker: %w", err)
		} else {
			_ = w.cmd.Wait()
		}
		_ = w.input.Close()
		_ = w.output.Close()
		close(w.stopped)
	})
	return w.stopErr
}

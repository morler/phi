package lisp

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNamespaceGlojureProcess(t *testing.T) {
	switch os.Getenv("PHI_DELEGATION_PROBE") {
	case "process-worker":
		rt, err := New()
		require.NoError(t, err)
		t.Setenv("PHI_DELEGATION_PROBE", "process-child")
		code := fmt.Sprintf(
			`(.Run (os:exec.Command %s "-test.run=^TestNamespaceGlojureProcess$"))`,
			strconv.Quote(os.Args[0]),
		)
		_, err = rt.Eval(t.Context(), code)
		require.NoError(t, err)
		return
	case "process-child":
		req, err := http.NewRequestWithContext(
			t.Context(),
			http.MethodGet,
			os.Getenv("PHI_DELEGATION_URL"),
			http.NoBody,
		)
		require.NoError(t, err)
		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		require.NoError(t, resp.Body.Close())
		require.NoError(t, os.WriteFile("process-finished.txt", []byte("finished"), 0o600))
		return
	}
	dir := t.TempDir()
	listener, err := (&net.ListenConfig{}).Listen(
		t.Context(),
		"tcp4",
		net.JoinHostPort(net.IPv4(127, 0, 0, 1).String(), "0"),
	)
	if errors.Is(err, syscall.EPERM) || errors.Is(err, syscall.EACCES) {
		t.Skipf("local listener unavailable: %v", err)
	}
	require.NoError(t, err)
	defer listener.Close()
	accepted, disconnected := make(chan struct{}), make(chan struct{})
	server := &http.Server{
		ReadHeaderTimeout: 5 * time.Second,
		Handler: http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
			close(accepted)
			<-r.Context().Done()
			close(disconnected)
		}),
	}
	go func() { _ = server.Serve(listener) }()
	defer server.Close()
	ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestNamespaceGlojureProcess$")
	cmd.Dir = dir
	cmd.Env = []string{
		"PHI_DELEGATION_PROBE=process-worker",
		"PHI_DELEGATION_URL=http://" + listener.Addr().String(),
		"HOME=" + dir,
		"TMPDIR=" + dir,
	}
	cmd.Stderr = os.Stderr
	cmd.SysProcAttr = namespaceAttributes()
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
	select {
	case <-accepted:
	case <-ctx.Done():
		require.FailNow(t, "Glojure child did not start", "%v", ctx.Err())
	}
	require.NoError(t, cmd.Process.Kill())
	require.Error(t, cmd.Wait())
	require.NoError(t, ctx.Err())
	select {
	case <-disconnected:
	case <-ctx.Done():
		require.FailNow(t, "Glojure child did not disconnect", "%v", ctx.Err())
	}
	_, err = os.Stat(filepath.Join(dir, "process-finished.txt"))
	assert.ErrorIs(t, err, os.ErrNotExist)
}

func TestNamespaceExternalDelegation(t *testing.T) {
	if os.Getenv("PHI_DELEGATION_PROBE") == "worker" {
		rt, err := New()
		require.NoError(t, err)
		_, err = rt.Eval(t.Context(), fmt.Sprintf(`(net:http.Get %s)`, strconv.Quote(os.Getenv("PHI_DELEGATION_URL"))))
		require.NoError(t, err)
		return
	}
	listener, err := (&net.ListenConfig{}).Listen(t.Context(), "tcp", "127.0.0.1:0")
	if errors.Is(err, syscall.EPERM) || errors.Is(err, syscall.EACCES) {
		t.Skipf("local listener unavailable: %v", err)
	}
	require.NoError(t, err)
	defer listener.Close()
	dir := t.TempDir()
	accepted, release, finished := make(chan struct{}), make(chan struct{}), make(chan error, 1)
	var releaseOnce sync.Once
	unblock := func() { releaseOnce.Do(func() { close(release) }) }
	server := &http.Server{
		ReadHeaderTimeout: 5 * time.Second,
		Handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			close(accepted)
			<-release
			finished <- os.WriteFile(filepath.Join(dir, "delegated.txt"), []byte("finished"), 0o600)
			w.WriteHeader(http.StatusNoContent)
		}),
	}
	go func() { _ = server.Serve(listener) }()
	defer func() {
		unblock()
		_ = server.Close()
	}()
	ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestNamespaceExternalDelegation$")
	cmd.Dir = dir
	cmd.Env = []string{
		"PHI_DELEGATION_PROBE=worker",
		"PHI_DELEGATION_URL=http://" + listener.Addr().String(),
		"HOME=" + dir,
		"TMPDIR=" + dir,
	}
	cmd.SysProcAttr = namespaceAttributes()
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
	select {
	case <-accepted:
	case <-ctx.Done():
		require.FailNow(t, "worker did not delegate the request", "%v", ctx.Err())
	}
	require.NoError(t, cmd.Process.Kill())
	require.Error(t, cmd.Wait())
	require.NoError(t, ctx.Err())
	unblock()
	select {
	case err = <-finished:
		require.NoError(t, err)
	case <-ctx.Done():
		require.FailNow(t, "delegated request did not finish", "%v", ctx.Err())
	}
	data, err := os.ReadFile(filepath.Join(dir, "delegated.txt"))
	require.NoError(t, err)
	assert.Equal(t, "finished", string(data))
}

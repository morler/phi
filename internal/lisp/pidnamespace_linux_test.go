package lisp

import (
	"bufio"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/sys/unix"
)

func TestWorkerSupervisorDeath(t *testing.T) {
	if os.Getenv("PHI_PROCESS_PROBE") == "namespace-supervisor" {
		cmd := exec.CommandContext(t.Context(), os.Args[0], "-test.run=^TestWorkerProcessGroup$")
		cmd.Env = append(os.Environ(), "PHI_PROCESS_PROBE=worker")
		cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
		cmd.SysProcAttr = namespaceAttributes()
		cmd.SysProcAttr.Pdeathsig = syscall.SIGKILL
		require.NoError(t, cmd.Start())
		<-t.Context().Done()
		return
	}
	probe := exec.CommandContext(t.Context(), os.Args[0], "-test.run=^$")
	probe.SysProcAttr = namespaceAttributes()
	if err := probe.Run(); err != nil {
		if errors.Is(err, syscall.EPERM) || errors.Is(err, syscall.EACCES) || errors.Is(err, syscall.ENOSYS) {
			t.Skipf("PID/user namespaces unavailable: %v", err)
		}
		require.NoError(t, err)
	}
	dir := t.TempDir()
	reader, writer, err := os.Pipe()
	require.NoError(t, err)
	defer reader.Close()
	defer writer.Close()
	ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestWorkerSupervisorDeath$")
	cmd.Dir = dir
	cmd.Env = []string{"PHI_PROCESS_PROBE=namespace-supervisor", "HOME=" + dir, "TMPDIR=" + dir}
	cmd.Stdin, cmd.Stderr = reader, os.Stderr
	stdout, err := cmd.StdoutPipe()
	require.NoError(t, err)
	require.NoError(t, cmd.Start())
	require.NoError(t, reader.Close())
	t.Cleanup(func() {
		if cmd.ProcessState == nil {
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
		}
	})
	lines := make(chan string, 3)
	go func() {
		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			lines <- scanner.Text()
		}
		close(lines)
	}()
	for _, prefix := range []string{"same ", "escaped ", "eval-ready"} {
		select {
		case line, ok := <-lines:
			require.True(t, ok, "namespace supervisor failed: %s", line)
			require.True(t, strings.HasPrefix(line, prefix), "%q", line)
		case <-ctx.Done():
			require.FailNow(t, "worker did not become ready", "%v", ctx.Err())
		}
	}
	require.NoError(t, cmd.Process.Kill())
	require.Error(t, cmd.Wait())
	require.NoError(t, ctx.Err())
	assertNamespaceTerminated(t, writer, dir)
}

func TestWorkerPIDNamespace(t *testing.T) {
	dir := t.TempDir()
	reader, writer, err := os.Pipe()
	require.NoError(t, err)
	defer reader.Close()
	defer writer.Close()
	ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestWorkerProcessGroup$")
	cmd.Dir = dir
	cmd.Env = []string{"PHI_PROCESS_PROBE=worker", "HOME=" + dir, "TMPDIR=" + dir}
	cmd.Stdin, cmd.Stderr = reader, os.Stderr
	cmd.SysProcAttr = namespaceAttributes()
	stdout, err := cmd.StdoutPipe()
	require.NoError(t, err)
	if err = cmd.Start(); err != nil {
		if errors.Is(err, syscall.EPERM) || errors.Is(err, syscall.EACCES) || errors.Is(err, syscall.ENOSYS) {
			t.Skipf("PID/user namespaces unavailable: %v", err)
		}
		require.NoError(t, err)
	}
	require.NoError(t, reader.Close())
	t.Cleanup(func() {
		if cmd.ProcessState == nil {
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
		}
	})
	lines := make(chan string, 3)
	go func() {
		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			lines <- scanner.Text()
		}
		close(lines)
	}()
	for _, prefix := range []string{"same ", "escaped ", "eval-ready"} {
		select {
		case line, ok := <-lines:
			require.True(t, ok, "worker exited before ready")
			require.True(t, strings.HasPrefix(line, prefix), "%q", line)
		case <-ctx.Done():
			require.FailNow(t, "worker did not become ready", "%v", ctx.Err())
		}
	}
	require.NoError(t, cmd.Process.Kill())
	require.Error(t, cmd.Wait())
	require.NoError(t, ctx.Err())
	assertNamespaceTerminated(t, writer, dir)
}

func namespaceAttributes() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{
		Cloneflags:                 syscall.CLONE_NEWUSER | syscall.CLONE_NEWPID,
		UidMappings:                []syscall.SysProcIDMap{{ContainerID: 0, HostID: os.Getuid(), Size: 1}},
		GidMappings:                []syscall.SysProcIDMap{{ContainerID: 0, HostID: os.Getgid(), Size: 1}},
		GidMappingsEnableSetgroups: false,
	}
}

func assertNamespaceTerminated(t *testing.T, writer *os.File, dir string) {
	t.Helper()
	fds := []unix.PollFd{{Fd: int32(writer.Fd()), Events: unix.POLLOUT}}
	require.Eventually(t, func() bool {
		_, pollErr := unix.Poll(fds, 0)
		return pollErr == nil && fds[0].Revents&unix.POLLERR != 0
	}, 5*time.Second, 10*time.Millisecond, "a descendant still holds the pipe open")
	_, err := writer.WriteString("go")
	assert.ErrorIs(t, err, syscall.EPIPE)
	for _, name := range []string{"same.txt", "escaped.txt"} {
		_, err = os.Stat(filepath.Join(dir, name))
		assert.ErrorIs(t, err, os.ErrNotExist)
	}
}

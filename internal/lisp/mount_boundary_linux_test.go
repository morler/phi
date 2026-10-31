package lisp

import (
	"context"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMountInheritedSocket(t *testing.T) {
	if os.Getenv("PHI_MOUNT_PROBE") == "inherited" {
		file := os.NewFile(3, "inherited")
		conn, err := net.FileConn(file)
		require.NoError(t, err)
		require.NoError(t, file.Close())
		_, err = conn.Write([]byte("delegated"))
		require.NoError(t, err)
		require.NoError(t, conn.Close())
		return
	}
	bwrap, err := exec.LookPath("bwrap")
	if err != nil {
		t.Skip("bubblewrap unavailable")
	}
	dir := t.TempDir()
	fds, err := syscall.Socketpair(syscall.AF_UNIX, syscall.SOCK_STREAM|syscall.SOCK_CLOEXEC, 0)
	require.NoError(t, err)
	host := os.NewFile(uintptr(fds[0]), "host")
	child := os.NewFile(uintptr(fds[1]), "child")
	defer host.Close()
	defer child.Close()
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
	defer cancel()
	cmd := exec.CommandContext(
		ctx,
		bwrap,
		"--unshare-user",
		"--unshare-pid",
		"--unshare-net",
		"--die-with-parent",
		"--ro-bind",
		"/",
		"/",
		"--proc",
		"/proc",
		"--dev",
		"/dev",
		"--tmpfs",
		dir,
		os.Args[0],
		"-test.run=^TestMountInheritedSocket$",
	)
	cmd.Env = []string{"PHI_MOUNT_PROBE=inherited"}
	cmd.ExtraFiles = []*os.File{child}
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "%s", out)
	data := make([]byte, len("delegated"))
	_, err = io.ReadFull(host, data)
	require.NoError(t, err)
	assert.Equal(t, "delegated", string(data))
}

func TestMountBoundary(t *testing.T) {
	switch os.Getenv("PHI_MOUNT_PROBE") {
	case "worker":
		rt, err := New()
		require.NoError(t, err)
		result, err := rt.Eval(t.Context(), `(nil? (os.WriteFile "shared.txt" (.getBytes "changed") 384))`)
		require.NoError(t, err)
		if os.Getenv("PHI_MOUNT_EXPECT") == "visible" {
			assert.Equal(t, "false", result.Value)
		} else {
			assert.Equal(t, "true", result.Value)
		}
		t.Setenv("PHI_MOUNT_PROBE", "connect")
		code := fmt.Sprintf(`(.Run (os:exec.Command %s "-test.run=^TestMountBoundary$"))`, strconv.Quote(os.Args[0]))
		_, err = rt.Eval(t.Context(), code)
		require.NoError(t, err)
		return
	case "connect":
		conn, err := (&net.Dialer{}).DialContext(t.Context(), "unix", os.Getenv("PHI_MOUNT_SOCKET"))
		if os.Getenv("PHI_MOUNT_EXPECT") == "visible" {
			require.NoError(t, err)
			require.NoError(t, conn.Close())
		} else {
			require.Error(t, err)
			if conn != nil {
				_ = conn.Close()
			}
		}
		return
	}
	bwrap, err := exec.LookPath("bwrap")
	if err != nil {
		t.Skip("bubblewrap unavailable")
	}
	dir := t.TempDir()
	shared := filepath.Join(dir, "shared.txt")
	require.NoError(t, os.WriteFile(shared, []byte("original"), 0o600))
	socket := filepath.Join(dir, "service.sock")
	listener, err := (&net.ListenConfig{}).Listen(t.Context(), "unix", socket)
	require.NoError(t, err)
	defer listener.Close()
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	for _, mode := range []string{"visible", "hidden"} {
		args := []string{
			"--unshare-user",
			"--unshare-pid",
			"--unshare-net",
			"--die-with-parent",
			"--ro-bind",
			"/",
			"/",
			"--proc",
			"/proc",
			"--dev",
			"/dev",
		}
		if mode == "hidden" {
			args = append(args, "--tmpfs", dir)
		}
		args = append(args, "--chdir", dir, os.Args[0], "-test.run=^TestMountBoundary$")
		cmd := exec.CommandContext(ctx, bwrap, args...)
		cmd.Env = []string{
			"PHI_MOUNT_PROBE=worker",
			"PHI_MOUNT_EXPECT=" + mode,
			"PHI_MOUNT_SOCKET=" + socket,
			"HOME=" + dir,
			"TMPDIR=" + dir,
		}
		out, runErr := cmd.CombinedOutput()
		require.NoError(t, runErr, "%s", out)
	}
	content, err := os.ReadFile(shared)
	require.NoError(t, err)
	assert.Equal(t, "original", string(content))
	require.NoError(t, listener.(*net.UnixListener).SetDeadline(time.Now().Add(time.Second)))
	conn, err := listener.Accept()
	require.NoError(t, err)
	require.NoError(t, conn.Close())
	require.NoError(t, listener.(*net.UnixListener).SetDeadline(time.Now().Add(100*time.Millisecond)))
	conn, err = listener.Accept()
	if conn != nil {
		_ = conn.Close()
	}
	var netErr net.Error
	require.ErrorAs(t, err, &netErr)
	assert.True(t, netErr.Timeout())
	assert.NotErrorIs(t, err, syscall.EBADF)
}

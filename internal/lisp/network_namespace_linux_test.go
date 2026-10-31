package lisp

import (
	"context"
	"errors"
	"fmt"
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

func TestNetworkNamespaceBoundary(t *testing.T) {
	switch os.Getenv("PHI_NETNS_PROBE") {
	case "tcp":
		rt, err := New()
		require.NoError(t, err)
		result, evalErr := rt.Eval(
			t.Context(),
			fmt.Sprintf(`(nil? (second (net:http.Get %s)))`, strconv.Quote(os.Getenv("PHI_NETNS_URL"))),
		)
		require.NoError(t, evalErr)
		assert.Equal(t, "false", result.Value)
		return
	case "unix":
		rt, err := New()
		require.NoError(t, err)
		t.Setenv("PHI_NETNS_PROBE", "unix-child")
		code := fmt.Sprintf(
			`(.Run (os:exec.Command %s "-test.run=^TestNetworkNamespaceBoundary$"))`,
			strconv.Quote(os.Args[0]),
		)
		_, err = rt.Eval(t.Context(), code)
		require.NoError(t, err)
		return
	case "unix-child":
		conn, err := (&net.Dialer{}).DialContext(t.Context(), "unix", os.Getenv("PHI_NETNS_SOCKET"))
		require.NoError(t, err)
		require.NoError(t, conn.Close())
		return
	case "abstract":
		conn, err := (&net.Dialer{}).DialContext(t.Context(), "unix", os.Getenv("PHI_NETNS_ABSTRACT"))
		require.Error(t, err)
		if conn != nil {
			_ = conn.Close()
		}
		return
	}
	dir := t.TempDir()
	tcp, err := (&net.ListenConfig{}).Listen(
		t.Context(),
		"tcp4",
		net.JoinHostPort(net.IPv4(127, 0, 0, 1).String(), "0"),
	)
	if errors.Is(err, syscall.EPERM) || errors.Is(err, syscall.EACCES) {
		t.Skipf("local TCP listener unavailable: %v", err)
	}
	require.NoError(t, err)
	defer tcp.Close()
	socket := filepath.Join(dir, "probe.sock")
	local, err := (&net.ListenConfig{}).Listen(t.Context(), "unix", socket)
	require.NoError(t, err)
	defer local.Close()
	abstractName := "@phi-netns-" + strconv.Itoa(os.Getpid())
	abstract, err := (&net.ListenConfig{}).Listen(t.Context(), "unix", abstractName)
	require.NoError(t, err)
	defer abstract.Close()
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
	defer cancel()
	for _, mode := range []string{"tcp", "unix", "abstract"} {
		cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestNetworkNamespaceBoundary$")
		cmd.Dir = dir
		cmd.Env = []string{
			"PHI_NETNS_PROBE=" + mode,
			"PHI_NETNS_URL=http://" + tcp.Addr().String(),
			"PHI_NETNS_SOCKET=" + socket,
			"PHI_NETNS_ABSTRACT=" + abstractName,
			"HOME=" + dir,
			"TMPDIR=" + dir,
		}
		cmd.SysProcAttr = namespaceAttributes()
		cmd.SysProcAttr.Cloneflags |= syscall.CLONE_NEWNET
		out, runErr := cmd.CombinedOutput()
		if errors.Is(runErr, syscall.EPERM) || errors.Is(runErr, syscall.EACCES) || errors.Is(runErr, syscall.ENOSYS) {
			t.Skipf("network/user namespaces unavailable: %v", runErr)
		}
		require.NoError(t, runErr, "%s", out)
	}
	require.NoError(t, tcp.(*net.TCPListener).SetDeadline(time.Now().Add(100*time.Millisecond)))
	conn, err := tcp.Accept()
	if conn != nil {
		_ = conn.Close()
	}
	var netErr net.Error
	require.ErrorAs(t, err, &netErr)
	assert.True(t, netErr.Timeout(), "unexpected TCP accept error: %v", err)
	require.NoError(t, local.(*net.UnixListener).SetDeadline(time.Now().Add(time.Second)))
	conn, err = local.Accept()
	require.NoError(t, err)
	require.NoError(t, conn.Close())
}

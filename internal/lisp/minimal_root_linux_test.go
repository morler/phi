package lisp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/sys/unix"
)

func TestMinimalRootRuntime(t *testing.T) {
	if os.Getenv("PHI_MINIMAL_ROOT") == "descendant" {
		require.NoError(t, syscall.Setpgid(0, 0))
		require.NoError(t, os.WriteFile("/tmp/descendant-ready", []byte(strconv.Itoa(os.Getpid())), 0o600))
		<-time.After(time.Minute)
		return
	}
	if os.Getenv("PHI_MINIMAL_ROOT") == "worker" {
		rt, err := New()
		require.NoError(t, err)
		result, err := rt.Eval(t.Context(), `(+ 20 22)`)
		require.NoError(t, err)
		assert.Equal(t, "42", result.Value)
		result, err = rt.Eval(
			t.Context(),
			fmt.Sprintf(`(nil? (second (os.ReadFile %s)))`, strconv.Quote(os.Getenv("PHI_SECRET_PATH"))),
		)
		require.NoError(t, err)
		assert.Equal(t, "false", result.Value)
		return
	}
	if os.Getenv("PHI_MINIMAL_ROOT") == "spawn" {
		rt, err := New()
		require.NoError(t, err)
		t.Setenv("PHI_MINIMAL_ROOT", "descendant")
		done := make(chan error, 1)
		go func() {
			_, evalErr := rt.Eval(
				t.Context(),
				`(.Run (os:exec.Command "/worker" "-test.run=^TestMinimalRootRuntime$"))`,
			)
			done <- evalErr
		}()
		var childPID []byte
		for {
			if childPID, err = os.ReadFile("/tmp/descendant-ready"); err == nil {
				break
			}
			select {
			case evalErr := <-done:
				require.NoError(t, evalErr)
				require.FailNow(t, "descendant exited before ready")
			default:
			}
			time.Sleep(time.Millisecond)
		}
		_, err = fmt.Fprintln(os.Stdout, "worker-ready", string(childPID))
		require.NoError(t, err)
		_, err = io.Copy(io.Discard, os.Stdin)
		require.NoError(t, err)
		return
	}
	bwrap, err := exec.LookPath("bwrap")
	if err != nil {
		t.Skip("bubblewrap unavailable")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 90*time.Second)
	defer cancel()
	dir := t.TempDir()
	bin := filepath.Join(dir, "worker")
	build := exec.CommandContext(ctx, "go", "test", "-c", "-o", bin, ".")
	build.Env = append(os.Environ(), "CGO_ENABLED=0")
	out, err := build.CombinedOutput()
	require.NoError(t, err, "%s", out)
	secret := filepath.Join(dir, "secret")
	require.NoError(t, os.WriteFile(secret, []byte("secret"), 0o600))
	args := []string{
		"--unshare-user",
		"--unshare-pid",
		"--unshare-net",
		"--unshare-ipc",
		"--die-with-parent",
		"--ro-bind",
		bin,
		"/worker",
		"--proc",
		"/proc",
		"--dev",
		"/dev",
		"--tmpfs",
		"/tmp",
		"--chdir",
		"/tmp",
		"/worker",
	}
	cmd := exec.CommandContext(ctx, bwrap, append(args, "-test.run=^TestMinimalRootRuntime$")...)
	cmd.Env = []string{"PHI_MINIMAL_ROOT=worker", "PHI_SECRET_PATH=" + secret, "HOME=/tmp", "TMPDIR=/tmp"}
	out, err = cmd.CombinedOutput()
	require.NoError(t, err, "%s", out)

	infoRead, infoWrite, err := os.Pipe()
	require.NoError(t, err)
	defer infoRead.Close()
	defer infoWrite.Close()
	workerArgs := append([]string{"--info-fd", "3"}, args...)
	cmd = exec.CommandContext(ctx, bwrap, append(workerArgs, "-test.run=^TestIsolatedRuntimeLifecycle$")...)
	cmd.Env = []string{"PHI_LIFECYCLE_PROBE=worker", "HOME=/tmp", "TMPDIR=/tmp"}
	cmd.ExtraFiles = []*os.File{infoWrite}
	input, err := cmd.StdinPipe()
	require.NoError(t, err)
	output, err := cmd.StdoutPipe()
	require.NoError(t, err)
	cmd.Stderr = io.Discard
	require.NoError(t, cmd.Start())
	require.NoError(t, infoWrite.Close())
	t.Cleanup(func() {
		if cmd.ProcessState == nil {
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
		}
	})
	_, err = fmt.Fprintln(input, `"(phi-lifecycle-wait)"`)
	require.NoError(t, err)
	ready := make(chan string, 1)
	go func() { line, _ := bufio.NewReader(output).ReadString('\n'); ready <- line }()
	select {
	case line := <-ready:
		require.Equal(t, `"running"`, strings.TrimSpace(line))
	case <-ctx.Done():
		require.FailNow(t, "isolated worker did not start", "%v", ctx.Err())
	}
	var info struct {
		ChildPID int `json:"child-pid"`
	}
	require.NoError(t, json.NewDecoder(infoRead).Decode(&info))
	require.Positive(t, info.ChildPID)
	children, err := os.ReadFile(fmt.Sprintf("/proc/%d/task/%d/children", info.ChildPID, info.ChildPID))
	require.NoError(t, err)
	fields := strings.Fields(string(children))
	require.Len(t, fields, 1, "%q", children)
	lifecyclePID, err := strconv.Atoi(fields[0])
	require.NoError(t, err)
	pidfd, err := unix.PidfdOpen(lifecyclePID, 0)
	require.NoError(t, err)
	defer unix.Close(pidfd)
	fds := []unix.PollFd{{Fd: int32(pidfd), Events: unix.POLLIN}}
	_, err = unix.Poll(fds, 0)
	require.NoError(t, err)
	require.Zero(t, fds[0].Revents, "isolated worker exited before cancellation")
	require.NoError(t, cmd.Process.Kill())
	require.Error(t, cmd.Wait())
	require.NoError(t, ctx.Err())
	fds = []unix.PollFd{{Fd: int32(pidfd), Events: unix.POLLIN}}
	require.Eventually(t, func() bool {
		_, pollErr := unix.Poll(fds, 0)
		return pollErr == nil && fds[0].Revents&unix.POLLIN != 0
	}, 5*time.Second, 10*time.Millisecond, "isolated worker did not exit")

	infoRead2, infoWrite2, err := os.Pipe()
	require.NoError(t, err)
	defer infoRead2.Close()
	defer infoWrite2.Close()
	workerCtx, cancelWorker := context.WithCancel(ctx)
	defer cancelWorker()
	cmd = exec.CommandContext(workerCtx, bwrap, append(workerArgs, "-test.run=^TestMinimalRootRuntime$")...)
	cmd.Env = []string{"PHI_MINIMAL_ROOT=spawn", "HOME=/tmp", "TMPDIR=/tmp"}
	cmd.ExtraFiles = []*os.File{infoWrite2}
	input, err = cmd.StdinPipe()
	require.NoError(t, err)
	defer input.Close()
	output, err = cmd.StdoutPipe()
	require.NoError(t, err)
	cmd.Stderr = io.Discard
	require.NoError(t, cmd.Start())
	spawnCmd := cmd
	t.Cleanup(func() {
		if spawnCmd.ProcessState == nil {
			_ = spawnCmd.Process.Kill()
			_ = spawnCmd.Wait()
		}
	})
	require.NoError(t, infoWrite2.Close())
	ready = make(chan string, 1)
	go func() { line, _ := bufio.NewReader(output).ReadString('\n'); ready <- line }()
	var descendantNSPID int
	select {
	case line := <-ready:
		_, err = fmt.Sscanf(line, "worker-ready %d", &descendantNSPID)
		require.NoError(t, err)
		require.Positive(t, descendantNSPID)
	case <-ctx.Done():
		require.FailNow(t, "Glojure descendant did not start", "%v", ctx.Err())
	}
	require.NoError(t, json.NewDecoder(infoRead2).Decode(&info))
	reaperPID := info.ChildPID
	children, err = os.ReadFile(fmt.Sprintf("/proc/%d/task/%d/children", reaperPID, reaperPID))
	require.NoError(t, err)
	fields = strings.Fields(string(children))
	require.Len(t, fields, 1, "%q", children)
	workerPID, err := strconv.Atoi(fields[0])
	require.NoError(t, err)
	tasks, err := os.ReadDir(fmt.Sprintf("/proc/%d/task", workerPID))
	require.NoError(t, err)
	fields = nil
	for _, task := range tasks {
		children, readErr := os.ReadFile(fmt.Sprintf("/proc/%d/task/%s/children", workerPID, task.Name()))
		if os.IsNotExist(readErr) {
			continue
		}
		require.NoError(t, readErr)
		fields = append(fields, strings.Fields(string(children))...)
	}
	require.Len(t, fields, 1, "%q", fields)
	descendantPID, err := strconv.Atoi(fields[0])
	require.NoError(t, err)
	status, err := os.ReadFile(fmt.Sprintf("/proc/%d/status", descendantPID))
	require.NoError(t, err)
	var namespacePIDs []string
	for line := range strings.SplitSeq(string(status), "\n") {
		if strings.HasPrefix(line, "NSpid:") {
			namespacePIDs = strings.Fields(line)[1:]
			break
		}
	}
	require.NotEmpty(t, namespacePIDs)
	assert.Equal(t, strconv.Itoa(descendantNSPID), namespacePIDs[len(namespacePIDs)-1])
	workerFD, err := unix.PidfdOpen(workerPID, 0)
	require.NoError(t, err)
	defer unix.Close(workerFD)
	descendantFD, err := unix.PidfdOpen(descendantPID, 0)
	require.NoError(t, err)
	defer unix.Close(descendantFD)
	for _, fd := range []int{workerFD, descendantFD} {
		fds = []unix.PollFd{{Fd: int32(fd), Events: unix.POLLIN}}
		_, err = unix.Poll(fds, 0)
		require.NoError(t, err)
		require.Zero(t, fds[0].Revents, "isolated process exited before cancellation")
	}
	cancelWorker()
	require.Error(t, cmd.Wait())
	require.ErrorIs(t, workerCtx.Err(), context.Canceled)
	require.NoError(t, ctx.Err())
	for _, fd := range []int{workerFD, descendantFD} {
		fds = []unix.PollFd{{Fd: int32(fd), Events: unix.POLLIN}}
		require.Eventually(t, func() bool {
			_, pollErr := unix.Poll(fds, 0)
			return pollErr == nil && fds[0].Revents&unix.POLLIN != 0
		}, 5*time.Second, 10*time.Millisecond, "isolated process did not exit")
	}
}

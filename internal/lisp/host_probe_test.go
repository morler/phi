package lisp

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
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

func TestHostProbe(t *testing.T) {
	if os.Getenv("PHI_HOST_PROBE") == "files" {
		rt, err := New()
		require.NoError(t, err)
		exe, err := os.Executable()
		require.NoError(t, err)
		cases := []struct{ name, code, value, failure string }{
			{"expression", `(+ 20 22)`, `42`, ""},
			{"define", `(defn probe-double [n] (* n 2))`, "", ""},
			{"call-definition", `(probe-double 21)`, `42`, ""},
			{"core-file", `(spit "core.txt" "core") (slurp "core.txt")`, "", "unable to resolve host form: gio.writer"},
			{"namespace-load", `(require '[glojure.go.io :as gio])`, "nil", ""},
			{
				"namespace-writer",
				`(with-open [w (gio/writer "namespace.txt")] (.Write w (.getBytes "namespace")))`,
				"",
				"EvalASTMaybeHostForm: gio/writer",
			},
			{"java-namespace", `(require '[clojure.java.io :as io])`, "", "not found in load path"},
			{"go-write", `(os.WriteFile "go.txt" (.getBytes "go") 384)`, "nil", ""},
			{"go-read", `(String. (first (os.ReadFile "fixture.txt")))`, `"fixture"`, ""},
			{
				"go-process",
				fmt.Sprintf(`(.Run (os:exec.Command %s "-test.run=^TestHostProbeHelper$"))`, strconv.Quote(exe)),
				"nil",
				"",
			},
			{"jvm-file", `(.mkdir (java.io.File. "jvm-dir"))`, "true", ""},
			{"jvm-runtime", `(Runtime/getRuntime)`, "", "EvalASTMaybeHostForm: Runtime/getRuntime"},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				result, evalErr := rt.Eval(t.Context(), tc.code)
				if tc.failure != "" {
					require.ErrorContains(t, evalErr, tc.failure)
					return
				}
				require.NoError(t, evalErr)
				if tc.value != "" {
					assert.Equal(t, tc.value, result.Value)
				}
			})
		}
		return
	}

	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "fixture.txt"), []byte("fixture"), 0o600))
	runHostProbe(t, dir, "files", "")
	for name, expected := range map[string]string{"go.txt": "go", "process.txt": "process"} {
		content, err := os.ReadFile(filepath.Join(dir, name))
		require.NoError(t, err)
		assert.Equal(t, expected, string(content))
	}
	info, err := os.Stat(filepath.Join(dir, "jvm-dir"))
	require.NoError(t, err)
	assert.True(t, info.IsDir())
}

func TestHostNetworkProbe(t *testing.T) {
	if os.Getenv("PHI_HOST_PROBE") == "network" {
		rt, err := New()
		require.NoError(t, err)
		addr := os.Getenv("PHI_PROBE_ADDR")
		_, err = rt.Eval(t.Context(), fmt.Sprintf(`(net.Dial "tcp" %s)`, strconv.Quote(addr)))
		require.ErrorContains(t, err, "unable to resolve symbol: net.Dial")
		_, port, err := net.SplitHostPort(addr)
		require.NoError(t, err)
		_, err = rt.Eval(t.Context(), fmt.Sprintf(`(java.net.Socket. "127.0.0.1" %s)`, port))
		require.ErrorContains(t, err, "unable to resolve symbol: java.net.Socket")
		result, err := rt.Eval(t.Context(), fmt.Sprintf(`(net:http.Get %s)`, strconv.Quote("http://"+addr)))
		require.NoError(t, err)
		assert.Contains(t, result.Value, "http.Response")
		return
	}

	listener, err := (&net.ListenConfig{}).Listen(t.Context(), "tcp", "127.0.0.1:0")
	if errors.Is(err, syscall.EPERM) || errors.Is(err, syscall.EACCES) {
		t.Skipf("local listener is unavailable: %v", err)
	}
	require.NoError(t, err)
	defer listener.Close()
	served := make(chan error, 1)
	go func() {
		conn, acceptErr := listener.Accept()
		if acceptErr != nil {
			served <- acceptErr
			return
		}
		defer conn.Close()
		_ = conn.SetDeadline(time.Now().Add(10 * time.Second))
		request, readErr := http.ReadRequest(bufio.NewReader(conn))
		if readErr != nil {
			served <- readErr
			return
		}
		_ = request.Body.Close()
		_, writeErr := fmt.Fprint(conn, "HTTP/1.1 200 OK\r\nContent-Length: 0\r\nConnection: close\r\n\r\n")
		served <- writeErr
	}()
	runHostProbe(t, t.TempDir(), "network", listener.Addr().String())
	select {
	case err := <-served:
		require.NoError(t, err)
	case <-time.After(10 * time.Second):
		require.FailNow(t, "local listener did not receive a request")
	}
}

func runHostProbe(t *testing.T, dir, mode, addr string) {
	t.Helper()
	exe, err := os.Executable()
	require.NoError(t, err)
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	name := "TestHostProbe"
	if mode == "network" {
		name = "TestHostNetworkProbe"
	}
	cmd := exec.CommandContext(ctx, exe, "-test.run=^"+name+"$", "-test.v")
	cmd.Dir = dir
	cmd.Env = []string{"PHI_HOST_PROBE=" + mode, "PHI_PROBE_ADDR=" + addr, "HOME=" + dir, "TMPDIR=" + dir}
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "%s", out)
}

func TestHostProbeHelper(t *testing.T) {
	if os.Getenv("PHI_HOST_PROBE") == "files" {
		require.NoError(t, os.WriteFile("process.txt", []byte("process"), 0o600))
	}
}

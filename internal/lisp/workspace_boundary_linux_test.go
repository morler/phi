package lisp

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/pulseaiclub/phi/internal/permission"
)

func TestReadonlyWorkspaceBypassesReadGate(t *testing.T) {
	if os.Getenv("PHI_WORKSPACE_PROBE") == "worker" {
		rt, err := New()
		require.NoError(t, err)
		for name, value := range map[string]string{"ordinary.txt": `"ordinary"`, "secret.txt": `"secret"`} {
			result, evalErr := rt.Eval(t.Context(), `(String. (first (os.ReadFile "`+name+`")))`)
			require.NoError(t, evalErr)
			assert.Equal(t, value, result.Value)
		}
		result, err := rt.Eval(t.Context(), `(nil? (os.WriteFile "ordinary.txt" (.getBytes "changed") 384))`)
		require.NoError(t, err)
		assert.Equal(t, "false", result.Value)
		return
	}
	bwrap, err := exec.LookPath("bwrap")
	if err != nil {
		t.Skip("bubblewrap unavailable")
	}
	workspace := t.TempDir()
	for name, value := range map[string]string{"ordinary.txt": "ordinary", "secret.txt": "secret"} {
		require.NoError(t, os.WriteFile(filepath.Join(workspace, name), []byte(value), 0o600))
	}
	policy := permission.DefaultPolicy()
	policy.SensitivePathDeny = []string{filepath.Join(workspace, "secret.txt")}
	gate, err := permission.NewGate(policy, workspace)
	require.NoError(t, err)
	decision, _ := gate.Check(
		t.Context(),
		permission.Request{Kind: permission.KindRead, Paths: []string{filepath.Join(workspace, "ordinary.txt")}},
	)
	require.Equal(t, permission.Allow, decision)
	decision, _ = gate.Check(
		t.Context(),
		permission.Request{Kind: permission.KindRead, Paths: []string{filepath.Join(workspace, "secret.txt")}},
	)
	require.Equal(t, permission.Deny, decision)
	ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
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
		"--chdir",
		workspace,
		os.Args[0],
		"-test.run=^TestReadonlyWorkspaceBypassesReadGate$",
	)
	cmd.Env = []string{"PHI_WORKSPACE_PROBE=worker", "HOME=" + workspace, "TMPDIR=" + workspace}
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "%s", out)
	content, err := os.ReadFile(filepath.Join(workspace, "ordinary.txt"))
	require.NoError(t, err)
	assert.Equal(t, "ordinary", string(content))
}

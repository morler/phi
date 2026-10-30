package util

import (
	"os"
	"os/exec"
	"strings"
)

// SetGitLocale makes git diagnostics stable for callers that classify errors.
func SetGitLocale(cmd *exec.Cmd) {
	cmd.Env = make([]string, 0, len(os.Environ())+1)
	for _, entry := range os.Environ() {
		if !strings.HasPrefix(entry, "LC_ALL=") && !strings.HasPrefix(entry, "LANG=") {
			cmd.Env = append(cmd.Env, entry)
		}
	}
	cmd.Env = append(cmd.Env, "LC_ALL=C", "LANG=C")
}

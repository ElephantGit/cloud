//go:build linux

package nodeservices

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// The preparation script downloads and checks the packages these tables later serve to Node; a
// digest edited on one side only would fail every Agent start after a successful preparation.
func TestAgentPackagesMatchPreparationScript(t *testing.T) {
	script, err := os.ReadFile(filepath.Join("..", "..", "scripts", "prepare-node.sh"))
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		name, key string
		table     map[string]AgentPackage
		host      func() (AgentPackage, error)
	}{{"OpenCode", "opencode_sha", openCodePackages, HostOpenCodePackage}, {"Claude Code", "claude_sha", claudePackages, HostClaudePackage}} {
		for arch, p := range c.table {
			if !targetLineHas(string(script), p.Target, c.key+"="+p.SHA256) {
				t.Errorf("%s %s: prepare-node.sh lacks %s=%s on the %s line", c.name, arch, c.key, p.SHA256, p.Target)
			}
		}
		if _, err := c.host(); err != nil {
			t.Errorf("%s: no pinned package for the test host %s: %v", c.name, runtime.GOARCH, err)
		}
	}
	if !strings.Contains(string(script), "claude_declaration_sha="+ClaudeDeclarationSHA256) {
		t.Error("prepare-node.sh does not check the pinned Claude Code declaration digest")
	}
}

// targetLineHas reports whether the script's case line for target also sets field.
func targetLineHas(script, target, field string) bool {
	for _, line := range strings.Split(script, "\n") {
		if strings.Contains(line, "target="+target+" ") {
			return strings.Contains(" "+line+" ", " "+field+" ")
		}
	}
	return false
}

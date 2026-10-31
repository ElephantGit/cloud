//go:build linux

package nodeservices

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// The preparation script downloads and checks the package this table later serves to Node; a
// digest edited on one side only would fail every Agent start after a successful preparation.
func TestOpenCodePackagesMatchPreparationScript(t *testing.T) {
	script, err := os.ReadFile(filepath.Join("..", "..", "scripts", "prepare-node.sh"))
	if err != nil {
		t.Fatal(err)
	}
	for arch, p := range openCodePackages {
		line := "target=" + p.Target + " plugin_sha=" + p.SHA256
		if !strings.Contains(string(script), line) {
			t.Errorf("%s: prepare-node.sh lacks %q", arch, line)
		}
	}
	if _, err := HostOpenCodePackage(); err != nil {
		t.Errorf("no pinned package for the test host %s: %v", runtime.GOARCH, err)
	}
}

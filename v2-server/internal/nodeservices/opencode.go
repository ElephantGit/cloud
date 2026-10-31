//go:build linux

package nodeservices

import (
	"errors"
	"runtime"
)

// OpenCodePackage is the pinned OpenCode 0.6.11 build Node installs on one Linux architecture.
// scripts/prepare-node.sh downloads the same release asset and checks the same digest.
type OpenCodePackage struct {
	Target string
	SHA256 string
}

// openCodePackages is keyed by runtime.GOARCH. A digest here is the trust anchor for the
// package bytes served to Node, so it is never read from the assets directory.
var openCodePackages = map[string]OpenCodePackage{
	"amd64": {Target: "x86_64-unknown-linux-gnu", SHA256: "88c00f81997fc7fe892f2d58a686f535678e562c7e000d33eb7ee3b1d0f754b5"},
	"arm64": {Target: "aarch64-unknown-linux-gnu", SHA256: "41f58c82f343153f23771f7f314c22212145b3dcf039451b5a869dba24894547"},
}

// HostOpenCodePackage returns the pinned build for the architecture this process runs on.
func HostOpenCodePackage() (OpenCodePackage, error) {
	p, ok := openCodePackages[runtime.GOARCH]
	if !ok {
		return OpenCodePackage{}, errors.New("no pinned OpenCode package for " + runtime.GOARCH)
	}
	return p, nil
}

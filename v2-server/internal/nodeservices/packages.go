//go:build linux

package nodeservices

import (
	"errors"
	"runtime"
)

// AgentPackage is one pinned Agent plugin build Node installs on one Linux architecture.
// scripts/prepare-node.sh downloads the same release asset and checks the same digest.
type AgentPackage struct {
	Target string
	SHA256 string
}

// The tables are keyed by runtime.GOARCH. A digest here is the trust anchor for the package
// bytes served to Node, so it is never read from the assets directory.
var (
	openCodePackages = map[string]AgentPackage{
		"amd64": {Target: "x86_64-unknown-linux-gnu", SHA256: "88c00f81997fc7fe892f2d58a686f535678e562c7e000d33eb7ee3b1d0f754b5"},
		"arm64": {Target: "aarch64-unknown-linux-gnu", SHA256: "41f58c82f343153f23771f7f314c22212145b3dcf039451b5a869dba24894547"},
	}
	claudePackages = map[string]AgentPackage{
		"amd64": {Target: "x86_64-unknown-linux-gnu", SHA256: "7e10e4b9c3564d17169ffd3dc289d3e60b0972516098275834203df66b9110f9"},
		"arm64": {Target: "aarch64-unknown-linux-gnu", SHA256: "6c2b69e9d880e9928f9bf3194b9b10232be190aa9f8919857d4fe0db645c1c3e"},
	}
)

// ClaudeDeclarationSHA256 is the digest of the Agent declaration inside the pinned Claude Code
// release (its manifest's declaration_sha256). Node re-proves it before handing out model access.
const ClaudeDeclarationSHA256 = "2d1afa5e03b476385a6c188f7444184be11e82e81191d8efe2e9f354456753c1"

// HostOpenCodePackage returns the pinned OpenCode 0.6.11 build for this process's architecture.
func HostOpenCodePackage() (AgentPackage, error) { return hostPackage(openCodePackages, "OpenCode") }

// HostClaudePackage returns the pinned Claude Code 0.4.5 build for this process's architecture.
func HostClaudePackage() (AgentPackage, error) { return hostPackage(claudePackages, "Claude Code") }

func hostPackage(table map[string]AgentPackage, name string) (AgentPackage, error) {
	p, ok := table[runtime.GOARCH]
	if !ok {
		return AgentPackage{}, errors.New("no pinned " + name + " package for " + runtime.GOARCH)
	}
	return p, nil
}

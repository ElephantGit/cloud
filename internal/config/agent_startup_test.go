package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// Startup policy must reach the server even when an operator's file predates the new key.
func TestAgentStartupBudgetDefaultsAndEnvironment(t *testing.T) {
	for _, tc := range []struct {
		name, yaml, environment string
		want                    time.Duration
	}{
		{"missing key", "", "", 30 * time.Minute},
		{"zero selects default", "issue_runs: {agent_start_timeout: 0s}\n", "", 30 * time.Minute},
		{"negative selects default", "issue_runs: {agent_start_timeout: -1m}\n", "", 30 * time.Minute},
		{"explicit budget", "issue_runs: {agent_start_timeout: 2m}\n", "", 2 * time.Minute},
		{"environment with absent key", "", "3m", 3 * time.Minute},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("CLOUD_ISSUE_RUNS_AGENT_START_TIMEOUT", tc.environment)
			path := filepath.Join(t.TempDir(), "config.yaml")
			yaml := "server: {read_timeout: 10s, write_timeout: 15s}\ndatabase: {conn_max_lifetime: 1h}\ncontrol: {grpc_addr: '127.0.0.1:8082'}\n" + tc.yaml
			if err := os.WriteFile(path, []byte(yaml), 0o600); err != nil {
				t.Fatal(err)
			}
			cfg, err := Load(path)
			if err != nil {
				t.Fatal(err)
			}
			if cfg.IssueRuns.AgentStartTimeout != tc.want {
				t.Fatalf("startup budget = %s, want %s", cfg.IssueRuns.AgentStartTimeout, tc.want)
			}
		})
	}
}

//go:build linux

package main

import (
	"errors"
	"fmt"
	"github.com/wanglongan587/cloud/v2-server/internal/agentruntime"
	"github.com/wanglongan587/cloud/v2-server/internal/execution"
	"github.com/wanglongan587/cloud/v2-server/internal/pluginmarket"
	"github.com/wanglongan587/cloud/v2-server/internal/store"
	"testing"
)

func TestSkillJobOutcome(t *testing.T) {
	for _, tc := range []struct {
		name string
		err  error
		want store.SkillJobOutcome
	}{
		{"success", nil, store.SkillJobSucceeded},
		{"download retry", fmt.Errorf("download: %w", pluginmarket.ErrTransient), store.SkillJobRetry},
		{"node exit", fmt.Errorf("node: %w", execution.ErrUnavailable), store.SkillJobRetry},
		{"archive rejected", pluginmarket.ErrPackage, store.SkillJobFailed},
		{"source rejected", pluginmarket.ErrSource, store.SkillJobFailed},
		{"unexpected", errors.New("unexpected"), store.SkillJobFailed},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := skillJobOutcome(tc.err); got != tc.want {
				t.Fatalf("skillJobOutcome(%v) = %d, want %d", tc.err, got, tc.want)
			}
		})
	}
}

func TestSkillFailureDetail(t *testing.T) {
	for _, tc := range []struct {
		name string
		err  error
		want string
	}{
		{"success", nil, ""},
		{"source", pluginmarket.ErrSource, "技能包下载地址没有通过公网 HTTPS 校验。请检查市场来源或本机 DNS 代理设置。"},
		{"package", pluginmarket.ErrPackage, "技能包下载失败或完整性校验未通过，请重新应用。"},
		{"credentials", pluginmarket.ErrCredentials, "市场来源凭据不可用，请由 owner 重新配置来源。"},
		{"node", fmt.Errorf("receipt: %w", agentruntime.ErrNodeRejected), "项目节点拒绝或未确认技能包操作，请查看服务日志后重试。"},
		{"unexpected", errors.New("unexpected"), "项目节点未确认操作，请查看服务日志后重试。"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := skillFailureDetail(tc.err); got != tc.want {
				t.Fatalf("skillFailureDetail(%v) = %q, want %q", tc.err, got, tc.want)
			}
		})
	}
}

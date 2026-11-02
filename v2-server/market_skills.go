//go:build linux

package main

import (
	"context"
	"errors"
	"github.com/wanglongan587/cloud/v2-server/internal/agentruntime"
	"github.com/wanglongan587/cloud/v2-server/internal/execution"
	"github.com/wanglongan587/cloud/v2-server/internal/pluginmarket"
	"github.com/wanglongan587/cloud/v2-server/internal/store"
	"log/slog"
	"os"
	"time"
)

func reconcileSkills(ctx context.Context, db *store.Store, driver *agentruntime.Driver) error {
	if driver != nil {
		online := map[string]bool{}
		for _, id := range driver.OnlineSkillProjects() {
			online[id] = true
		}
		jobs, e := db.SkillJobs(ctx)
		if e != nil {
			return e
		}
		for _, j := range jobs {
			if !online[j.Project] {
				continue
			}
			ok, e := db.StartSkillJob(ctx, j)
			if e != nil {
				return e
			}
			if !ok {
				continue
			}
			e = applySkill(ctx, driver, j)
			if ctx.Err() != nil {
				return ctx.Err()
			}
			if errors.Is(e, agentruntime.ErrOfflineSkillNode) {
				continue
			}
			outcome := skillJobOutcome(e)
			failure := skillFailureDetail(e)
			if e != nil {
				slog.Warn("skill job failed",
					"project", j.Project,
					"selection", j.Selection,
					"plugin", j.Namespace+"/"+j.Identifier,
					"version", j.Version,
					"desired", j.Desired,
					"outcome", outcome,
					"failure", failure,
					"error", e,
				)
			}
			if err := db.FinishSkillJob(ctx, j, outcome, failure); err != nil {
				return err
			}
		}
	}
	return db.RefreshSkillStates(ctx)
}

func skillJobOutcome(e error) store.SkillJobOutcome {
	switch {
	case e == nil:
		return store.SkillJobSucceeded
	case errors.Is(e, pluginmarket.ErrTransient), errors.Is(e, execution.ErrUnavailable):
		return store.SkillJobRetry
	default:
		return store.SkillJobFailed
	}
}

func skillFailureDetail(e error) string {
	switch {
	case e == nil:
		return ""
	case errors.Is(e, pluginmarket.ErrSource):
		return "技能包下载地址没有通过公网 HTTPS 校验。请检查市场来源或本机 DNS 代理设置。"
	case errors.Is(e, pluginmarket.ErrPackage):
		return "技能包下载失败或完整性校验未通过，请重新应用。"
	case errors.Is(e, pluginmarket.ErrCredentials):
		return "市场来源凭据不可用，请由 owner 重新配置来源。"
	case errors.Is(e, agentruntime.ErrNodeRejected):
		return "项目节点拒绝或未确认技能包操作，请查看服务日志后重试。"
	default:
		return "项目节点未确认操作，请查看服务日志后重试。"
	}
}

func applySkill(ctx context.Context, d *agentruntime.Driver, j store.SkillJob) error {
	remove := j.Desired == "removed"
	version := j.Version
	var file string
	if remove && j.Previous != "" {
		version = j.Previous
	}
	if !remove {
		var creds pluginmarket.Credentials
		if len(j.Credentials) > 0 {
			var e error
			creds, e = pluginmarket.OpenCredentials(j.Credentials, j.Space+"/"+j.Namespace+"/"+j.Retrieval.Endpoint+"/"+j.Retrieval.Bucket)
			if e != nil {
				return e
			}
		}
		var e error
		file, e = pluginmarket.Download(ctx, j.Locator, j.Digest, j.Retrieval, creds)
		if e != nil {
			return e
		}
		defer os.Remove(file)
	}
	return d.ApplySkill(ctx, j.Project, j.Namespace+"/"+j.Identifier, version, file, j.Digest, remove)
}
func runSkillReconciler(ctx context.Context, db *store.Store, d *agentruntime.Driver) {
	ticker := time.NewTicker(3 * time.Second)
	defer ticker.Stop()
	for {
		if e := reconcileSkills(ctx, db, d); e != nil && ctx.Err() == nil {
			slog.Warn("skill reconciliation failed", "error", e)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

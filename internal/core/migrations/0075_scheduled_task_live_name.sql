-- 删除定时任务后释放名称：名称只在同一租户、issue、创建者的未删除定义里唯一，与工作流归档后释放
-- 名称（workflow_name_uniq）一致。0061 的唯一约束连软删除墓碑一起约束，这里换成部分唯一索引。
-- 既有数据已满足更严格的旧约束，建索引不会失败；墓碑与既有行都不改写。
ALTER TABLE scheduled_tasks DROP CONSTRAINT IF EXISTS scheduled_tasks_tenant_id_issue_id_creator_user_id_name_key;
CREATE UNIQUE INDEX IF NOT EXISTS scheduled_tasks_live_name_uniq
  ON scheduled_tasks(tenant_id, issue_id, creator_user_id, name) WHERE deleted_at IS NULL;

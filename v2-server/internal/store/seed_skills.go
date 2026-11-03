package store

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// Starter skills are real catalog entries, not browser fixtures. They are inserted once per
// account and then behave exactly like user-created skills: editable, deletable and materialized
// into new Agent sessions.
var starterSkills = []SkillInput{
	{
		Name:        "code-review",
		Description: "审查改动中的正确性、风险、兼容性和测试缺口。",
		Content:     "# 代码审查\n\n## 目标\n\n在合并前发现会影响行为、安全、兼容性或可维护性的具体问题。\n\n## 步骤\n\n1. 先阅读需求和相关测试，确认改动想解决什么。\n2. 检查正常路径、失败路径、边界条件和并发/恢复语义。\n3. 核对新增依赖、配置、迁移和外部副作用。\n4. 只报告能从代码或运行结果证明的问题，并给出文件位置和影响。\n\n## 输出\n\n按严重程度列出问题；没有问题时说明检查过的范围和剩余风险。",
	},
	{
		Name:        "test-plan",
		Description: "把功能改动拆成可执行的正常、边界、失败和回归检查。",
		Content:     "# 测试计划\n\n## 目标\n\n用最少的用例覆盖改动的真实风险，而不是只复述实现步骤。\n\n## 步骤\n\n1. 写出验收标准和前置条件。\n2. 覆盖正常路径、空数据、权限、失败与恢复。\n3. 标出需要真实数据库、浏览器或外部服务验证的边界。\n4. 为每个用例记录实际结果、证据位置和未覆盖风险。\n\n## 输出\n\n用表格列出用例、预期、实际结果和证据。",
	},
	{
		Name:        "release-notes",
		Description: "把一组改动整理成用户能够理解并采取行动的发布说明。",
		Content:     "# 发布说明\n\n## 目标\n\n说明改了什么、谁会受影响，以及需要采取什么行动。\n\n## 步骤\n\n1. 按用户可感知的变化归类，而不是照搬提交记录。\n2. 标明兼容性、迁移、限制和已知问题。\n3. 删除内部实现细节和无法验证的承诺。\n\n## 输出\n\n- 本次变化\n- 使用方式\n- 升级或迁移步骤\n- 已知限制",
	},
}

// seedStarterSkills claims the account's one-time seed marker before inserting the catalog. A
// concurrent login loses the marker race and inserts nothing; a transaction failure rolls both
// the marker and any partial catalog back together.
func seedStarterSkills(ctx context.Context, tx pgx.Tx, tenant, user string) error {
	tag, err := tx.Exec(ctx, `
		INSERT INTO v2_skill_seeds (user_id) VALUES ($1)
		ON CONFLICT DO NOTHING`, user)
	if err != nil {
		return fmt.Errorf("claim starter skills: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return nil
	}
	for _, skill := range starterSkills {
		if _, err := tx.Exec(ctx, `
			INSERT INTO v2_skills (tenant_id, user_id, name, description, content)
			VALUES ($1, $2, $3, $4, $5)`, tenant, user, skill.Name, skill.Description, skill.Content); err != nil {
			return fmt.Errorf("insert starter skill %s: %w", skill.Name, err)
		}
	}
	return nil
}

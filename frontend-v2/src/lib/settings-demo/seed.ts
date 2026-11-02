import type { Role, Snapshot } from './types'

/** Seeds the roles that used to be hard-coded in the workflow demo catalog. */
export function seedRoles(now = new Date().toISOString()): Role[] {
  return [
    {
      id: 'role-reviewer',
      namespace: 'local',
      name: '代码审查',
      description: '检查改动的正确性、风险和测试覆盖',
      content:
        '# 代码审查\n\n你负责审查改动的正确性、风险和测试覆盖。\n\n- 先复述改动目标和影响范围。\n- 检查边界条件、错误路径和回归风险。\n- 给出可执行的修改建议，并标明未验证的部分。',
      updatedAt: now,
    },
    {
      id: 'role-writer',
      namespace: 'local',
      name: '技术写作',
      description: '把结论整理成清楚的说明文档',
      content:
        '# 技术写作\n\n你负责把技术结论整理成清晰、可执行的说明。\n\n- 先说明读者和目标。\n- 用结论先行组织内容。\n- 区分事实、推断和未知项。',
      updatedAt: now,
    },
  ]
}

export function seed(): Snapshot {
  const now = new Date().toISOString()
  return {
    schema: 4,
    revision: 1,
    connections: [
      {
        id: 'demo-connection',
        name: '团队日常模型 · 示例',
        protocol: 'openai-completions',
        authMode: 'bearer',
        baseUrl: 'https://models.example.com/v1',
        enabled: true,
        credentialConfigured: true,
        models: [
          {
            id: 'demo/general',
            name: '通用模型 · 示例',
            contextWindow: 128000,
            maxTokens: 8192,
          },
        ],
        version: 1,
        createdAt: now,
        updatedAt: now,
      },
    ],
    modelDefault: {
      connectionId: 'demo-connection',
      modelId: 'demo/general',
      version: 1,
    },
    skills: [
      {
        id: 'local-summary',
        namespace: 'local',
        name: 'change-summary',
        description: '把一组改动整理成便于阅读的更新说明。',
        content:
          '# 更新说明\n\n## 目标\n\n说明改了什么、为什么改，以及用户需要知道的影响。\n\n## 输出格式\n\n- 本次变化\n- 使用方式\n- 已知限制\n\n避免复制实现流水账。',
        source: { kind: 'local' },
        availability: 'available',
      },
      {
        id: 'local-test-plan',
        namespace: 'local',
        name: 'test-plan',
        description: '为功能改动整理正常路径、边界情况与回归检查。',
        content:
          '# 测试计划\n\n1. 明确验收标准。\n2. 覆盖正常、空数据与失败场景。\n3. 记录实际结果。',
        source: { kind: 'local' },
        availability: 'available',
      },
    ],
    roles: seedRoles(now),
  }
}

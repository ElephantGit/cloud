import type {
  MemberListItem,
  PluginCatalogEntry,
  SpacePlugin,
} from '@/api/generated.schemas'
import {
  DEMO_SPACE,
  DEMO_TENANT,
  pluginKey,
  type DemoUser,
  type Skill,
  type Snapshot,
} from './types'

export const pluginSkill = (pluginId: string): Skill => ({
  id: `plugin-skill-${pluginId}`,
  namespace: 'demo',
  name: pluginId.endsWith('writing-kit') ? 'writing-guide' : 'review-checklist',
  description: pluginId.endsWith('writing-kit')
    ? '让文档清晰、简洁，保持术语和语气一致。'
    : '从正确性、边界条件与回归风险三个角度检查改动。',
  content: pluginId.endsWith('writing-kit')
    ? '# 写作指南\n\n先明确读者和目的，再组织内容。\n\n## 工作步骤\n\n1. 用一句话说明结论。\n2. 每段只讨论一个主题。\n3. 给出可执行的下一步。\n\n> 此文档来自演示插件，不会注入任何真实会话。'
    : '# 代码审查清单\n\n- 对照需求确认行为。\n- 检查空值、错误和并发边界。\n- 说明证据，区分缺陷和建议。\n\n## 输出\n\n提供问题位置、影响和修复建议。\n\n> 仅用于设置页面的演示。',
  source: { kind: 'plugin', pluginId },
  availability: 'available',
})

export function seed(user: DemoUser): Snapshot {
  const now = new Date().toISOString()
  const member = (
    id: string,
    name: string,
    role: string,
    status = 'active',
  ): MemberListItem => ({
    id: `membership-${id}`,
    userId: id,
    displayName: name,
    role,
    status,
    version: 1,
    tenantId: DEMO_TENANT,
    joinMethod: 'invitation',
    joinedAt: now,
    actorDisplayName: user.displayName,
  })
  const entries: [
    string,
    string,
    PluginCatalogEntry['kind'],
    string,
    string,
  ][] = [
    [
      'writing-kit',
      '写作工具箱',
      'skill',
      '从提纲到成稿，让团队文档保持清晰与一致。',
      '1.2.0',
    ],
    [
      'github-tools',
      'GitHub 工具',
      'mcp',
      '查阅仓库、拉取请求与代码上下文。',
      '2.1.0',
    ],
    [
      'review-kit',
      '代码审查指南',
      'skill',
      '一份可复用的检查清单，关注正确性与回归风险。',
      '1.0.0',
    ],
    [
      'browser-tools',
      '浏览器工具',
      'mcp',
      '在浏览器中查看页面结构、检查界面和整理信息。',
      '0.9.0',
    ],
    [
      'web-preview',
      '网页预览',
      'webview',
      '在独立视图中阅读网页与预览内容。',
      '1.0.0',
    ],
    [
      'starter-pack',
      '团队入门合集',
      'pack',
      '查看适合新团队的插件组合；合集安装尚不支持。',
      '1.0.0',
    ],
  ]
  const catalog = entries.map(
    (
      [identifier, title, kind, description, version],
      index,
    ): PluginCatalogEntry => ({
      id: `catalog-${index}`,
      sourceNamespace: 'demo',
      identifier,
      title,
      kind,
      description,
      version,
      homepage: null,
      license: 'MIT',
      logo: null,
      url: null,
      sha256: null,
      targets: null,
      packMembers:
        kind === 'pack' ? ['demo/writing-kit', 'demo/github-tools'] : null,
      readme: `# ${title}\n\n${description}\n\n## 关于此示例\n\n这是 ORA Settings 的演示资源。安装、移除仅改变本浏览器中的演示状态，不下载软件、不连接外部服务。\n\n## 使用范围\n\n来源：演示目录。版本：${version}。\n\n插件的运行与配置能力尚未接入。`,
      marketplaceVisible: true,
      sourceUrl: 'https://example.invalid/ora-demo-marketplace',
      indexedAt: now,
    }),
  )
  const installations = catalog.slice(0, 2).map((entry): SpacePlugin => ({
    id: `selection-${entry.identifier}`,
    tenantId: DEMO_TENANT,
    spaceId: DEMO_SPACE,
    identifier: entry.identifier,
    sourceNamespace: entry.sourceNamespace,
    kind: entry.kind,
    desiredState: 'installed',
    desiredVersion: entry.version,
    observedState: 'installed',
    observedVersion: entry.version,
    installError: null,
    version: 1,
    createdAt: now,
    updatedAt: now,
  }))
  installations.push({
    ...installations[0],
    id: 'selection-browser-tools',
    identifier: 'browser-tools',
    kind: 'mcp',
    desiredVersion: '0.9.0',
    observedVersion: null,
    observedState: 'failed',
    installError: '演示安装失败：示例包暂不可用。可重试以完成本地演示。',
  })
  const members = [
    member(user.id, user.displayName, 'admin'),
    member('demo-lin', '林沐 · 示例', 'admin'),
    member('demo-chen', '陈予 · 示例', 'member'),
    member('demo-zhou', '周宁 · 示例', 'member', 'disabled'),
  ]
  return {
    schema: 1,
    revision: 1,
    members,
    history: members.map((m) => ({
      id: `history-${m.userId}`,
      tenantId: DEMO_TENANT,
      userId: m.userId,
      method: m.userId === user.id ? 'workspace_created' : 'invitation',
      actorUserId: user.id,
      actorDisplayName: user.displayName,
      joinedAt: now,
      invitationNote: '初始演示成员',
    })),
    links: [
      {
        id: 'demo-application-link',
        kind: 'apply',
        tenantId: DEMO_TENANT,
        createdBy: user.id,
        createdByName: user.displayName,
        createdAt: now,
        expiresAt: new Date(Date.now() + 7 * 86400000).toISOString(),
        note: '团队协作申请 · 示例',
        version: 1,
      },
    ],
    requests: [
      {
        id: 'demo-request',
        tenantId: DEMO_TENANT,
        userId: 'demo-xu',
        displayName: '许安 · 示例',
        linkId: 'demo-application-link',
        linkNote: '团队协作申请 · 示例',
        status: 'pending',
        createdAt: now,
        version: 1,
      },
    ],
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
    catalog,
    installations,
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
      pluginSkill(pluginKey(catalog[0])),
    ],
  }
}

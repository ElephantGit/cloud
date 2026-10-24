import type { TranslationBundle } from '@/i18n/resource-bundle'

/** Publish admission guidance shared by the fault summary and the problem list. */
export const publishErrorTranslations = {
  'zh-CN': {
    'workflows.inspector.field.structuredOutputHint':
      'Cloud 当前不支持结构化输出；开启后无法发布。关闭此设置可使用普通文本输出。',
    'workflows.errors.graphInvalid':
      '工作流配置未通过发布检查，请检查节点、连线和变量配置后重新发布。',
    'workflows.errors.graphProblems': '工作流配置未通过发布检查，请修正以下问题后重新发布。',
    'workflows.errors.problemNode': '节点 {{node}}',
    'workflows.errors.problemCheck': '请检查此处的节点、连线或变量配置。',
    'workflows.errors.structuredOutputUnsupported':
      'Cloud 当前不支持结构化输出，请关闭该节点的「结构化输出」后重新发布。',
    'workflows.errors.configUnsupported': 'Cloud 当前不支持此配置，请关闭或移除后重新发布。',
    'workflows.errors.agentSchemaVersionUnsupported':
      'Cloud 只支持 Agent 配置版本 3，请使用兼容的工作流文档。',
    'workflows.errors.nodeKindUnsupported': 'Cloud 当前不支持此节点类型，请替换或移除该节点。',
  },
  'en-US': {
    'workflows.inspector.field.structuredOutputHint':
      'Cloud does not support structured output yet; enabling it prevents publishing. Turn it off to use plain text output.',
    'workflows.errors.graphInvalid':
      'The workflow cannot be published. Check its nodes, connections and variables, then publish again.',
    'workflows.errors.graphProblems':
      'The workflow cannot be published. Fix the following problems and publish again.',
    'workflows.errors.problemNode': 'Node {{node}}',
    'workflows.errors.problemCheck': 'Check the node, connection or variable configuration here.',
    'workflows.errors.structuredOutputUnsupported':
      'Cloud does not support structured output yet. Turn off Structured output on this node and publish again.',
    'workflows.errors.configUnsupported':
      'Cloud does not support this setting yet. Turn it off or remove it, then publish again.',
    'workflows.errors.agentSchemaVersionUnsupported':
      'Cloud only supports Agent configuration version 3. Use a compatible workflow document.',
    'workflows.errors.nodeKindUnsupported':
      'Cloud does not support this node type yet. Replace or remove the node.',
  },
} satisfies TranslationBundle

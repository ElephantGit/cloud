/**
 * Messages for the Loop container and its early-exit marker.
 *
 * They live beside the feature's main bundle for the same reason the history and launch-field
 * bundles do: the group is self-contained — the container's palette identity, the starter body the
 * factory builds, the inspector fields, and the `loopExit` marker — and keeping it here stops the
 * main bundle outgrowing the line budget that keeps its keys scannable. `translations.ts` spreads
 * both locales in, so the feature still ships one bundle.
 */
export const loopTranslations = {
  'zh-CN': {
    'workflows.node.loop.label': '循环',
    'workflows.node.loop.description': '重复执行直到满足条件',
    'workflows.node.loopExit.label': '早退',
    'workflows.node.loopExit.description': '循环体内的终端节点：被选中时提前结束循环',
    'workflows.loop.roundStart': '轮次开始',
    'workflows.loop.loopAgent': '循环 Agent',
    'workflows.loop.promptTemplate': '改进当前值并输出新值：{{#NODE_ID.value#}}',
    'workflows.loop.field.maxIterations': '最大轮次',
    'workflows.loop.field.maxIterationsBehavior': '轮次耗尽时',
    'workflows.loop.behavior.fail': '失败',
    'workflows.loop.behavior.succeed': '导出最后一轮并成功',
    'workflows.loop.field.initialValue': '初始值',
    'workflows.loop.defaultBehavior':
      '每轮将 Agent 输出反馈为下一轮的 value；输出非空时结束，并导出为 result。',
    'workflows.loop.legacyUnsupported': '此旧循环节点缺少可执行配置，请删除后重新添加。',
  },
  'en-US': {
    'workflows.node.loop.label': 'Loop',
    'workflows.node.loop.description': 'Repeat until the exit condition is met',
    'workflows.node.loopExit.label': 'Early exit',
    'workflows.node.loopExit.description':
      'Terminal node inside a Loop body that ends the loop when selected',
    'workflows.loop.roundStart': 'Round start',
    'workflows.loop.loopAgent': 'Loop Agent',
    'workflows.loop.promptTemplate':
      'Improve the current value and output the new value: {{#NODE_ID.value#}}',
    'workflows.loop.field.maxIterations': 'Maximum rounds',
    'workflows.loop.field.maxIterationsBehavior': 'When rounds run out',
    'workflows.loop.behavior.fail': 'Fail',
    'workflows.loop.behavior.succeed': 'Publish the final round',
    'workflows.loop.field.initialValue': 'Initial value',
    'workflows.loop.defaultBehavior':
      'Each Agent output feeds the next round as value; a non-empty output stops the Loop and is exported as result.',
    'workflows.loop.legacyUnsupported':
      'This legacy Loop lacks executable configuration. Delete it and add a new Loop.',
  },
}

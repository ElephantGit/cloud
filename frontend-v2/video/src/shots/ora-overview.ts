import type { Shot } from '../record/shot.ts'

/** The film's first chapter, about 20 s: what Ora is, before the scenes show it at work. */
export const oraOverview: Shot = {
  name: 'ora-overview',
  scene: null,
  play(d) {
    d.slide({
      eyebrow: '01 / Ora Cloud',
      title: '人和 Agent，一个团队',
      narration: 'Ora 把人和 Agent 放进同一个团队，一起把事情做完。',
      items: [
        { title: '工作流', body: '把研发经验固化下来，提升日常工作效率和质量', narration: '工作流把研发经验固化下来。' },
        { title: '协作看板', body: '复杂任务拆开，分给人，也分给 Agent', narration: '协作看板把任务分给人和 Agent。' },
        { title: '定时任务', body: '例行检查按时自动跑，结果等人确认', narration: '定时任务让例行检查自动运行。' },
        { title: 'Dashboard', body: '整个空间的进度和消耗，一眼看清', narration: 'Dashboard 看清整个空间的进度和消耗。' },
      ],
    })
  },
}

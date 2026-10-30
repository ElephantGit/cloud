import { Cable, Monitor, Puzzle, Sparkles, Users } from 'lucide-react'

export const settingsPages = [
  { path: 'members', title: '成员', icon: Users },
  { path: 'model-connections', title: '模型连接', icon: Cable },
  { path: 'theme', title: '主题', icon: Monitor },
  { path: 'plugins', title: 'Plugins', icon: Puzzle },
  { path: 'skills', title: 'Skills', icon: Sparkles },
] as const

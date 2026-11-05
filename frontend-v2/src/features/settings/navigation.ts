import { Bot, Cable, Monitor, Puzzle, Sparkles, UsersRound } from 'lucide-react'

export const settingsPages = [
  { path: 'squads', title: '小队', icon: UsersRound },
  { path: 'roles', title: '角色', icon: Bot },
  { path: 'model-connections', title: '模型连接', icon: Cable },
  { path: 'theme', title: '主题', icon: Monitor },
  { path: 'plugins', title: 'Plugins', icon: Puzzle },
  { path: 'skills', title: 'Skills', icon: Sparkles },
] as const

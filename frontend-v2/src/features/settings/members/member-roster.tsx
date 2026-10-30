import type { MemberListItem } from '@/api/generated.schemas'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { NativeSelect, NativeSelectOption } from '@/components/ui/native-select'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'

export function MemberRoster({
  members,
  currentUserId,
  canManage,
  pending,
  onRole,
  onRemove,
  onHistory,
}: {
  members: MemberListItem[]
  currentUserId: string
  canManage: boolean
  pending: boolean
  onRole: (member: MemberListItem, role: 'admin' | 'member') => void
  onRemove: (member: MemberListItem) => void
  onHistory: (member: MemberListItem) => void
}) {
  return (
    <div className="overflow-hidden rounded-lg border">
      <Table>
        <TableHeader>
          <TableRow>
            <TableHead>成员</TableHead>
            <TableHead>角色</TableHead>
            <TableHead>状态</TableHead>
            {canManage && <TableHead className="text-right">管理</TableHead>}
          </TableRow>
        </TableHeader>
        <TableBody>
          {members.map((member) => (
            <TableRow key={member.userId}>
              <TableCell className="py-4">
                <div className="flex items-center gap-3">
                  <span
                    className="flex size-8 shrink-0 items-center justify-center rounded-full bg-muted text-xs"
                    aria-hidden="true"
                  >
                    {member.displayName.slice(0, 1)}
                  </span>
                  <div>
                    <span className="text-sm font-medium">
                      {member.displayName}
                    </span>
                    {member.userId === currentUserId && (
                      <span className="ml-2 text-xs text-muted-foreground">
                        你
                      </span>
                    )}
                    <p className="mt-1 text-xs text-muted-foreground">
                      {member.userId === currentUserId
                        ? '当前登录用户的演示身份'
                        : '演示工作空间成员'}
                    </p>
                  </div>
                </div>
              </TableCell>
              <TableCell>
                {canManage && member.status === 'active' ? (
                  <NativeSelect
                    aria-label={`调整 ${member.displayName} 的角色`}
                    value={member.role}
                    disabled={pending}
                    onChange={(e) =>
                      onRole(
                        member,
                        e.target.value === 'admin' ? 'admin' : 'member',
                      )
                    }
                  >
                    <NativeSelectOption value="admin">
                      管理员
                    </NativeSelectOption>
                    <NativeSelectOption value="member">成员</NativeSelectOption>
                  </NativeSelect>
                ) : (
                  <span className="text-sm">
                    {member.role === 'admin' ? '管理员' : '成员'}
                  </span>
                )}
              </TableCell>
              <TableCell>
                <Badge
                  variant={member.status === 'active' ? 'secondary' : 'outline'}
                >
                  {member.status === 'active' ? '有效' : '已停用'}
                </Badge>
              </TableCell>
              {canManage && (
                <TableCell>
                  <div className="flex justify-end gap-1">
                    <Button
                      variant="ghost"
                      size="sm"
                      onClick={() => onHistory(member)}
                    >
                      加入历史
                    </Button>
                    {member.status === 'active' && (
                      <Button
                        variant="ghost"
                        size="sm"
                        disabled={pending}
                        onClick={() => onRemove(member)}
                      >
                        停用
                      </Button>
                    )}
                  </div>
                </TableCell>
              )}
            </TableRow>
          ))}
        </TableBody>
      </Table>
    </div>
  )
}

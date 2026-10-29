import { useState } from 'react'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { useAddHuaweiMember, usePeopleSearch } from '@/features/members/api'
import { faultCode } from '@/lib/api-client'

/** Searches Tianzhou through Ora and rechecks the selected employee on add. */
export function HuaweiAdd({ tenantId }: { tenantId: string }) {
  const [keyword, setKeyword] = useState('')
  const [role, setRole] = useState<'admin' | 'member'>('member')
  const people = usePeopleSearch(tenantId, keyword)
  const add = useAddHuaweiMember(tenantId)
  return (
    <section className="space-y-3 rounded-md border p-4">
      <h2 className="font-medium">从华为人员目录添加</h2>
      <div className="flex gap-2">
        <Input
          aria-label="搜索姓名或工号"
          value={keyword}
          onChange={(event) => setKeyword(event.target.value)}
          placeholder="姓名或工号，至少两个字符"
        />
        <Select
          value={role}
          onValueChange={(value) => setRole(value === 'admin' ? 'admin' : 'member')}
        >
          <SelectTrigger className="w-28" aria-label="新成员角色">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="member">成员</SelectItem>
            <SelectItem value="admin">管理员</SelectItem>
          </SelectContent>
        </Select>
      </div>
      {people.isError && <p className="text-sm text-destructive">人员目录暂不可用，请稍后再试</p>}
      {people.data?.items.map((person) => (
        <div key={person.globalUserId} className="flex items-center gap-2 border-b py-2 text-sm">
          <span className="min-w-0 flex-1">
            {person.name} · {person.employeeNumber} · {person.departmentName}
          </span>
          <Button
            size="sm"
            disabled={add.isPending}
            onClick={() => add.mutate({ person, keyword, role })}
          >
            添加
          </Button>
        </div>
      ))}
      {add.isError && (
        <p className="text-xs text-destructive">添加失败：{faultCode(add.error) ?? 'unknown'}</p>
      )}
    </section>
  )
}

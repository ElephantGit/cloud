import { useMutation, useQuery } from '@tanstack/react-query'
import { Button } from '@/components/ui/button'
import { getProviders, startLogin } from '@/lib/auth-api'

const LABELS: Record<string, string> = { dev: '开发者登录', github: '使用 GitHub 登录', 'huawei-idaas': '使用华为账号登录' }

/** One button per login provider the backend offers. */
export function LoginScreen({ returnTo }: { returnTo: string }) {
  const providers = useQuery({ queryKey: ['auth', 'providers'], queryFn: getProviders, retry: false })
  const login = useMutation({
    mutationFn: (provider: string) => startLogin(provider, returnTo),
    onSuccess: (url) => window.location.assign(url),
  })

  return (
    <div className="flex h-svh items-center justify-center bg-muted/40">
      <div className="flex w-80 flex-col gap-4 rounded-xl border bg-background p-6 shadow-sm">
        <div className="flex items-center gap-2">
          <div className="flex size-8 items-center justify-center rounded-lg bg-primary text-sm font-semibold text-primary-foreground">O</div>
          <h1 className="text-lg font-medium">登录 Ora</h1>
        </div>
        {providers.isPending && <p className="text-sm text-muted-foreground">加载中…</p>}
        {providers.isError && <p className="text-sm text-destructive">无法获取登录方式，请稍后重试。</p>}
        {providers.data?.providers.map((p) => (
          <Button key={p} disabled={login.isPending} onClick={() => login.mutate(p)}>
            {LABELS[p] ?? p}
          </Button>
        ))}
        {login.isError && <p className="text-sm text-destructive">登录失败，请重试。</p>}
      </div>
    </div>
  )
}

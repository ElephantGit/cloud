import { Navigate, type RouteObject } from 'react-router'
import { features } from './features'
import { Layout } from './layout'

export const routes: RouteObject[] = [
  {
    path: '/',
    element: <Layout />,
    // Shown while the first lazily loaded page arrives.
    hydrateFallbackElement: (
      <p className="p-4 text-sm text-muted-foreground">加载中…</p>
    ),
    children: [
      {
        index: true,
        element: <Navigate to={`/${features[0].path}`} replace />,
      },
      ...features.map((f) => ({
        path: f.path,
        children: f.children,
        lazy: async () => ({ Component: (await f.page()).default }),
      })),
      { path: '*', element: <Navigate to="/" replace /> },
    ],
  },
]

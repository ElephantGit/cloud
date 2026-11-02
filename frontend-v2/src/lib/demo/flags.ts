/**
 * Whether the demo mock layer is on.
 *
 * `VITE_DEMO_MOCK=1` turns it on for a demo run; unset or any other value keeps every page on real
 * data, so `npm run check` and `npm run e2e` never depend on mock content. This is the only place
 * that reads the variable: a demo command sets it once and every mock module follows.
 */
export const demoMockEnabled = (): boolean => import.meta.env.VITE_DEMO_MOCK === '1'

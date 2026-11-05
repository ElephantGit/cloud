/**
 * The register of demo mocks for the 11-06 recording.
 *
 * Every mock lives in this directory and is on only when `demoMockEnabled()` (see `flags.ts`) says
 * so; pages never import mock data directly, they ask a module here. Each mock module is listed
 * below with the backend that replaces it. The change that lands that backend deletes the module
 * and its entry together, so this list is always the set of screens the recording still fakes.
 * `registry.test.ts` fails when a module is added without an entry or an entry outlives its module.
 */
export interface DemoMock {
  /** The module in this directory. */
  module: string
  /** The recording scene whose screens show it. */
  scene: 'newcomer' | 'lead' | 'ops'
  /** What the mock stands in for. */
  covers: string
  /** The backend change that replaces it and deletes the module. */
  replacedBy: string
}

export const demoMocks: readonly DemoMock[] = []

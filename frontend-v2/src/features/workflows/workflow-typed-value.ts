type Obj = Record<string, unknown>
const obj = (value: unknown): Obj => value && typeof value === 'object' && !Array.isArray(value) ? value as Obj : {}
const str = (value: unknown) => typeof value === 'string' ? value : ''

function fileReference(value: unknown): Obj | null {
  const raw = typeof value === 'string' ? value : str(obj(value).path)
  if (!raw || /^(?:[A-Za-z]:|[\\/])/.test(raw)) return null
  const parts = raw.split(/[\\/]+/).filter((part) => part && part !== '.')
  if (!parts.length || parts.some((part) => part === '..' || /^(?:con|prn|aux|nul|com[1-9]|lpt[1-9])(?:\..*)?$/i.test(part))) return null
  return { kind: 'workspace_file', path: parts.join('/') }
}
export function parseTypedValue(text: string, type: string): { valid: boolean; value?: unknown } {
  if (!text) return { valid: true, value: undefined }
  if (type === 'string' || type === 'secret') return { valid: true, value: text }
  if (type === 'file') { const value = fileReference(text); return value ? { valid: true, value } : { valid: false } }
  let value: unknown
  try { value = JSON.parse(text) } catch { return type === 'any' ? { valid: true, value: text } : { valid: false } }
  const record = (v: unknown) => v !== null && typeof v === 'object' && !Array.isArray(v)
  const array = (v: unknown, test: (item: unknown) => boolean) => Array.isArray(v) && v.every(test)
  const valid = type === 'any' || (type === 'number' && typeof value === 'number' && Number.isFinite(value)) || (type === 'integer' && typeof value === 'number' && Number.isInteger(value)) || (type === 'boolean' && typeof value === 'boolean') || (type === 'object' && record(value)) || (['array', 'array[any]'].includes(type) && Array.isArray(value)) || (type === 'array[string]' && array(value, (item) => typeof item === 'string')) || (type === 'array[number]' && array(value, (item) => typeof item === 'number' && Number.isFinite(item))) || (type === 'array[boolean]' && array(value, (item) => typeof item === 'boolean')) || (type === 'array[object]' && array(value, record)) || (type === 'array[file]' && array(value, (item) => fileReference(item) !== null))
  if (!valid) return { valid: false }
  if (type === 'array[file]' && Array.isArray(value)) value = value.map(fileReference)
  return { valid: true, value }
}
export function formatTypedValue(value: unknown, type: string): string {
  if (value === undefined) return ''
  if (type === 'file' && obj(value).kind === 'workspace_file') return str(obj(value).path)
  if (type === 'array[file]' && Array.isArray(value)) return JSON.stringify(value.map((item) => obj(item).kind === 'workspace_file' ? obj(item).path : item))
  return typeof value === 'string' ? value : JSON.stringify(value)
}

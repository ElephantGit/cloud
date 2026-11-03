import type { Shot } from '../record/shot.ts'
import { leadDecompose } from './lead-decompose.ts'

/** Every shot the recorder knows, by name. */
export const shots: Record<string, Shot> = {
  [leadDecompose.name]: leadDecompose,
}

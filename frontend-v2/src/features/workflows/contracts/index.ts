// The subset of desktop's `@ora/contracts` the workflow editor imports: the generated DTO types,
// copied unchanged, and the error the local client throws in place of a remote contract error.
export type * from "./dto/workflow.ts";
export type * from "./dto/skill.ts";
export type * from "./dto/plugin.ts";
export { ContractError } from "./error.ts";

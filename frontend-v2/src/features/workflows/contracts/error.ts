/**
 * A refused operation, shaped like desktop's `RemoteContractError`: a stable `code` the editor
 * localizes through `errors.<code>`, and the parameters that message interpolates.
 */
export class ContractError extends Error {
  readonly code: string;
  readonly params: Record<string, unknown>;

  constructor(code: string, params: Record<string, unknown> = {}) {
    super(code);
    this.name = "ContractError";
    this.code = code;
    this.params = params;
  }
}

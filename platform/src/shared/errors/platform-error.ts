export type PlatformErrorOptions = {
  code: string;
  detail: string;
  status: number;
  title: string;
  retryAfterSeconds?: number;
  type?: string;
};

export class PlatformError extends Error {
  readonly code: string;
  readonly status: number;
  readonly title: string;
  readonly type: string;
  readonly retryAfterSeconds?: number;

  constructor(options: PlatformErrorOptions) {
    super(options.detail);
    this.name = "PlatformError";
    this.code = options.code;
    this.status = options.status;
    this.title = options.title;
    this.retryAfterSeconds = options.retryAfterSeconds;
    this.type = options.type ?? `https://file.cheap/problems/${options.code}`;
  }
}

export class CatalogPreconditionError extends Error {
  constructor() {
    super("The catalog changed while it was being updated");
    this.name = "CatalogPreconditionError";
  }
}

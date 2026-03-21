export class BoxerError extends Error {
  override name = "BoxerError";

  constructor(message: string) {
    super(message);
    // Restore prototype chain for instanceof checks across transpile boundaries
    Object.setPrototypeOf(this, new.target.prototype);
  }
}

export class BoxerValidationError extends BoxerError {
  override name = "BoxerValidationError";
}

export class BoxerAPIError extends BoxerError {
  override name = "BoxerAPIError";

  constructor(
    message: string,
    public readonly statusCode: number,
  ) {
    super(message);
    Object.setPrototypeOf(this, new.target.prototype);
  }
}

export class BoxerTimeoutError extends BoxerAPIError {
  override name = "BoxerTimeoutError";

  constructor(message: string, statusCode: number) {
    super(message, statusCode);
    Object.setPrototypeOf(this, new.target.prototype);
  }
}

export class BoxerOutputLimitError extends BoxerAPIError {
  override name = "BoxerOutputLimitError";

  constructor(message: string, statusCode: number) {
    super(message, statusCode);
    Object.setPrototypeOf(this, new.target.prototype);
  }
}

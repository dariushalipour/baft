// src/auth/domain/errors.ts — Auth domain error types

export class DomainError extends Error {
  public readonly code: string;
  constructor(code: string, message: string) {
    super(message);
    this.name = "DomainError";
    this.code = code;
  }
}

export class ValidationError extends DomainError {
  public readonly field: string;
  constructor(field: string, message: string) {
    super("validation_error", `${field}: ${message}`);
    this.name = "ValidationError";
    this.field = field;
  }
}

export class NotFoundError extends DomainError {
  constructor(resource: string) {
    super("not_found", `${resource} not found`);
    this.name = "NotFoundError";
  }
}

export class ConflictError extends DomainError {
  constructor(message: string) {
    super("conflict", message);
    this.name = "ConflictError";
  }
}

// lib/billing/domain/errors.dart — Billing domain error types

class DomainError implements Exception {
  final String code;
  final String message;

  DomainError({required this.code, required this.message});

  @override
  String toString() => 'DomainError[$code]: $message';
}

class ValidationError extends DomainError {
  final String field;

  ValidationError({required String field, required String message})
      : field = field,
        super(code: 'validation_error', message: '$field: $message');
}

class NotFoundError extends DomainError {
  NotFoundError(String resource)
      : super(code: 'not_found', message: '$resource not found');
}

class ConflictError extends DomainError {
  ConflictError(String message)
      : super(code: 'conflict', message: message);
}

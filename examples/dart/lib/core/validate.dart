class ValidationException implements Exception {
  final String field;
  final String message;

  ValidationException({required this.field, required this.message});

  @override
  String toString() => '$field: $message';
}

void nonEmpty(String value, String field) {
  if (value.trim().isEmpty) {
    throw ValidationException(field: field, message: 'must not be empty');
  }
}

void positiveNumber(int value, String field) {
  if (value <= 0) {
    throw ValidationException(field: field, message: 'must be a positive number');
  }
}

void validEmail(String email) {
  if (!RegExp(r'^[^\s@]+@[^\s@]+\.[^\s@]+$').hasMatch(email)) {
    throw ValidationException(field: 'email', message: 'must be a valid email address');
  }
}

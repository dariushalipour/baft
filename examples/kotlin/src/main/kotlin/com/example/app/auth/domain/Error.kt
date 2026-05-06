package com.example.app.auth.domain

class DomainError(
    val code: String,
    message: String,
) : RuntimeException(message) {
    override fun toString(): String = "DomainError[$code]: $message"
}

class ValidationError(
    val field: String,
    message: String,
) : DomainError("validation_error", "$field: $message") {
    override fun toString(): String = "ValidationError[$field]: $message"
}

class NotFoundError(
    resource: String,
) : DomainError("not_found", "$resource not found") {
    override fun toString(): String = "NotFoundError: $resource not found"
}

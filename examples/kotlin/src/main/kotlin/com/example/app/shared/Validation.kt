package com.example.app.shared

import java.util.UUID

fun generateUUID(): String = UUID.randomUUID().toString()

fun String.requireNonBlank(field: String): Unit {
    if (this.isBlank()) {
        throw com.example.app.domain.ValidationError(field, field, "must not be blank")
    }
}

fun Int.requirePositive(field: String): Unit {
    if (this <= 0) {
        throw com.example.app.domain.ValidationError(field, field, "must be positive")
    }
}

fun String.requireEmail(field: String): Unit {
    if (!this.matches(Regex("^[^@]+@[^@]+\\.[^@]+$"))) {
        throw com.example.app.domain.ValidationError(field, field, "must be a valid email")
    }
}

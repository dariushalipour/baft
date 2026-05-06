package com.example.app.auth.application

import com.example.app.auth.domain.User
import com.example.app.auth.domain.UserRepository
import com.example.app.auth.domain.DomainError
import com.example.app.auth.domain.NotFoundError

class JwtService(
    private val userRepo: UserRepository,
) {
    fun generateToken(user: User): String {
        return "fake-jwt-token-for-${user.base.id}"
    }

    fun validateToken(token: String): User? {
        // In a real app, this would decode and verify the JWT
        return null
    }

    fun requireAuth(token: String?): User {
        if (token == null || token.isEmpty()) {
            throw DomainError("unauthorized", "Authorization header required")
        }
        val user = validateToken(token)
            ?: throw NotFoundError("token")
        return user
    }
}

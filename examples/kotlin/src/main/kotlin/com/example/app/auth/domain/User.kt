package com.example.app.auth.domain

import java.time.Instant

data class BaseEntity(
    val id: String,
    val createdAt: Instant,
    val updatedAt: Instant,
)

data class User(
    val base: BaseEntity,
    val email: String,
    val name: String,
    val role: UserRole,
)

enum class UserRole { ADMIN, MEMBER, VIEWER }

interface UserRepository {
    fun findById(id: String): User?
    fun findByEmail(email: String): User?
    fun create(user: User): User
}

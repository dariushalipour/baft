package com.example.app.billing.domain

import java.time.Instant

enum class OrderStatus { PENDING, CONFIRMED, SHIPPED, DELIVERED, CANCELLED }

data class OrderItem(
    val productId: String,
    val quantity: Int,
    val unitPriceCents: Int,
)

data class Order(
    val id: String,
    val userId: String,
    val items: List<OrderItem>,
    val status: OrderStatus,
    val totalCents: Int,
    val createdAt: Instant,
    val updatedAt: Instant,
)

interface OrderRepository {
    fun findById(id: String): Order?
    fun save(order: Order): Unit
    fun listByUser(userId: String, limit: Int): List<Order>
}

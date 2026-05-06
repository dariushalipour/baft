package com.example.app.billing.application

import com.example.app.billing.domain.Order
import com.example.app.billing.domain.OrderItem
import com.example.app.billing.domain.OrderRepository
import com.example.app.billing.domain.OrderStatus
import com.example.app.billing.domain.ValidationError
import java.time.Instant

data class CreateOrderInput(
    val userId: String,
    val items: List<OrderItemInput>,
)

data class OrderItemInput(
    val productId: String,
    val quantity: Int,
)

data class CreateOrderOutput(
    val order: Order,
)

class CreateOrderUseCase(
    private val orderRepo: OrderRepository,
) {
    fun execute(input: CreateOrderInput): CreateOrderOutput {
        if (input.userId.trim().isEmpty) {
            throw ValidationError("userId", "must not be empty")
        }

        if (input.items.isEmpty()) {
            throw ValidationError("items", "at least one item is required")
        }

        for ((i, item) in input.items.withIndex()) {
            if (item.productId.trim().isEmpty) {
                throw ValidationError("items[$i].productId", "must not be empty")
            }
            if (item.quantity <= 0) {
                throw ValidationError("items[$i].quantity", "must be a positive number")
            }
        }

        val order = Order(
            id = "order-${System.currentTimeMillis()}",
            userId = input.userId,
            items = input.items.map { OrderItem(it.productId, it.quantity, 0) },
            status = OrderStatus.PENDING,
            totalCents = 0,
            createdAt = Instant.now(),
            updatedAt = Instant.now(),
        )

        orderRepo.save(order)

        return CreateOrderOutput(order)
    }
}

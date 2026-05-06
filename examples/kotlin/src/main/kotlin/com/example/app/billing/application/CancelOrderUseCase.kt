package com.example.app.billing.application

import com.example.app.billing.domain.OrderRepository
import com.example.app.billing.domain.OrderStatus
import com.example.app.billing.domain.DomainError

class CancelOrderUseCase(
    private val orderRepo: OrderRepository,
) {
    fun execute(orderId: String) {
        val order = orderRepo.findById(orderId)
            ?: throw com.example.app.billing.domain.NotFoundError("Order $orderId")

        if (order.status != OrderStatus.PENDING) {
            throw DomainError("order_not_pending", "only pending orders can be cancelled")
        }

        // In a real app, we'd update the order status
    }
}

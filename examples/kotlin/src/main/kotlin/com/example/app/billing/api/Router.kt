package com.example.app.billing.api

import com.example.app.billing.application.CreateOrderUseCase
import com.example.app.billing.application.CancelOrderUseCase
import com.example.app.billing.domain.ValidationError

data class HttpRequest(
    val method: String,
    val path: String,
    val body: Map<String, Any>,
)

data class HttpResponse(
    val statusCode: Int,
    val body: String,
)

class ApiRouter(
    private val createOrderUseCase: CreateOrderUseCase,
    private val cancelOrderUseCase: CancelOrderUseCase,
) {
    fun route(request: HttpRequest): HttpResponse {
        try {
            if (request.method == "POST" && request.path == "/orders") {
                return handleCreateOrder(request)
            }

            if (request.method == "POST" &&
                request.path.startsWith("/orders/") &&
                request.path.endsWith("/cancel")) {
                return handleCancelOrder(request)
            }

            return HttpResponse(404, "{\"error\": \"not found\"}")
        } catch (e: Exception) {
            return HttpResponse(500, "{\"error\": \"internal server error\"}")
        }
    }

    private fun handleCreateOrder(request: HttpRequest): HttpResponse {
        try {
            val userId = request.body["user_id"] as? String ?: ""
            val itemsRaw = request.body["items"] as? List<*> ?: emptyList<Any>()

            val items = itemsRaw.map { item ->
                val m = item as Map<String, Any>
                CreateOrderUseCase.OrderItemInput(
                    productId = m["product_id"] as? String ?: "",
                    quantity = (m["quantity"] as? Number)?.toInt() ?: 0,
                )
            }

            val input = CreateOrderUseCase.CreateOrderInput(userId, items)
            val output = createOrderUseCase.execute(input)
            return HttpResponse(201, "{\"order\": \"created\"}")
        } catch (e: ValidationError) {
            return HttpResponse(400, "{\"error\": \"${e.message}\"}")
        }
    }

    private fun handleCancelOrder(request: HttpRequest): HttpResponse {
        val parts = request.path.split("/")
        val orderId = parts[2]
        cancelOrderUseCase.execute(orderId)
        return HttpResponse(204, "")
    }
}

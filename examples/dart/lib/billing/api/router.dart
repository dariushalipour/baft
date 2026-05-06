// lib/billing/api/router.dart — Billing HTTP router

import '../usecase/create_order.dart';
import '../usecase/cancel_order.dart';
import '../domain/errors.dart';

class HttpRequest {
  final String method;
  final String path;
  final Map<String, dynamic> body;

  HttpRequest({
    required this.method,
    required this.path,
    required this.body,
  });
}

class HttpResponse {
  final int statusCode;
  final String body;

  HttpResponse({required this.statusCode, required this.body});
}

class ApiRouter {
  final CreateOrderUseCase createOrder;
  final CancelOrderUseCase cancelOrder;

  ApiRouter({
    required this.createOrder,
    required this.cancelOrder,
  });

  Future<HttpResponse> route(HttpRequest request) async {
    try {
      if (request.method == 'POST' && request.path == '/orders') {
        return await _handleCreateOrder(request);
      }

      if (request.method == 'POST' &&
          request.path.startsWith('/orders/') &&
          request.path.endsWith('/cancel')) {
        return await _handleCancelOrder(request);
      }

      return HttpResponse(statusCode: 404, body: '{"error": "not found"}');
    } catch (e) {
      return HttpResponse(
        statusCode: 500,
        body: '{"error": "internal server error"}',
      );
    }
  }

  Future<HttpResponse> _handleCreateOrder(HttpRequest request) async {
    try {
      final input = CreateOrderInput(
        userId: request.body['user_id'] as String? ?? '',
        items: (request.body['items'] as List?)
                ?.map((item) => OrderItemInput(
                      productId: item['product_id'] as String? ?? '',
                      quantity: (item['quantity'] as num?)?.toInt() ?? 0,
                    ))
                .toList() ??
            [],
      );
      await createOrder.execute(input);
      return HttpResponse(
        statusCode: 201,
        body: '{"order": "created"}',
      );
    } on ValidationError catch (e) {
      return HttpResponse(statusCode: 400, body: '{"error": ${e.message}}');
    }
  }

  Future<HttpResponse> _handleCancelOrder(HttpRequest request) async {
    final parts = request.path.split('/');
    final orderId = parts[2];
    await cancelOrder.execute(orderId);
    return HttpResponse(statusCode: 204, body: '');
  }
}

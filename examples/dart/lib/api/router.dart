// lib/api/router.dart — Root-level API router

import '../auth/usecase/jwt.dart';
import '../billing/api/router.dart' as billing;
import '../core/logger.dart';

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
  final billing.ApiRouter billingRouter;
  final JwtService jwtService;
  final Logger logger;

  ApiRouter({
    required this.billingRouter,
    required this.jwtService,
    required this.logger,
  });

  Future<HttpResponse> route(HttpRequest request) async {
    try {
      if (request.method == 'POST' && (request.path == '/orders' || request.path.startsWith('/orders/'))) {
        final token = request.body['authorization'] as String? ?? '';
        await jwtService.requireAuth(token);
        final billingRequest = billing.HttpRequest(
          method: request.method,
          path: request.path,
          body: request.body,
        );
        final billingResponse = await billingRouter.route(billingRequest);
        return HttpResponse(
          statusCode: billingResponse.statusCode,
          body: billingResponse.body,
        );
      }

      return HttpResponse(statusCode: 404, body: '{"error": "not found"}');
    } catch (e) {
      logger.error('Route error', context: {'path': request.path, 'error': e.toString()});
      return HttpResponse(
        statusCode: 500,
        body: '{"error": "internal server error"}',
      );
    }
  }
}

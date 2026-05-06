// lib/billing/usecase/cancel_order.dart — Use case for cancelling orders

import '../domain/order.dart';
import '../domain/errors.dart';

class CancelOrderUseCase {
  final OrderRepository _repo;

  CancelOrderUseCase(this._repo);

  Future<void> execute(String orderId) async {
    final order = await _repo.findById(orderId);
    if (order == null) {
      throw NotFoundError('Order $orderId');
    }

    if (order.status != OrderStatus.pending) {
      throw DomainError(
        code: 'order_not_pending',
        message: 'only pending orders can be cancelled',
      );
    }

    // In a real app, we'd update the order status
  }
}

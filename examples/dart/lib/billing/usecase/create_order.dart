// lib/billing/usecase/create_order.dart — Use case for creating orders

import '../domain/order.dart';
import '../domain/errors.dart';

class CreateOrderInput {
  final String userId;
  final List<OrderItemInput> items;

  CreateOrderInput({required this.userId, required this.items});
}

class OrderItemInput {
  final String productId;
  final int quantity;

  OrderItemInput({required this.productId, required this.quantity});
}

class CreateOrderOutput {
  final Order order;

  CreateOrderOutput({required this.order});
}

class CreateOrderUseCase {
  final OrderRepository _repo;

  CreateOrderUseCase(this._repo);

  Future<CreateOrderOutput> execute(CreateOrderInput input) async {
    if (input.userId.trim().isEmpty) {
      throw ValidationError(field: 'userId', message: 'must not be empty');
    }

    if (input.items.isEmpty) {
      throw ValidationError(field: 'items', message: 'at least one item is required');
    }

    for (var i = 0; i < input.items.length; i++) {
      final item = input.items[i];
      if (item.productId.trim().isEmpty) {
        throw ValidationError(field: 'items[$i].productId', message: 'must not be empty');
      }
      if (item.quantity <= 0) {
        throw ValidationError(field: 'items[$i].quantity', message: 'must be a positive number');
      }
    }

    final order = Order(
      id: 'order-${DateTime.now().millisecondsSinceEpoch}',
      userId: input.userId,
      items: input.items.map((item) => OrderItem(
        productId: item.productId,
        quantity: item.quantity,
        unitPriceCents: 0,
      )).toList(),
      status: OrderStatus.pending,
      totalCents: 0,
      createdAt: DateTime.now(),
      updatedAt: DateTime.now(),
    );

    await _repo.save(order);

    return CreateOrderOutput(order: order);
  }
}

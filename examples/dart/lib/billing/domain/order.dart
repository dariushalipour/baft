// lib/billing/domain/order.dart — Billing domain entities

enum OrderStatus { pending, confirmed, shipped, delivered, cancelled }

class OrderItem {
  final String productId;
  final int quantity;
  final int unitPriceCents;

  OrderItem({
    required this.productId,
    required this.quantity,
    required this.unitPriceCents,
  });
}

class Order {
  final String id;
  final String userId;
  final List<OrderItem> items;
  final OrderStatus status;
  final int totalCents;
  final DateTime createdAt;
  final DateTime updatedAt;

  Order({
    required this.id,
    required this.userId,
    required this.items,
    required this.status,
    required this.totalCents,
    required this.createdAt,
    required this.updatedAt,
  });
}

abstract class OrderRepository {
  Future<Order?> findById(String id);
  Future<void> save(Order order);
  Future<List<Order>> listByUser(String userId, int limit);
}

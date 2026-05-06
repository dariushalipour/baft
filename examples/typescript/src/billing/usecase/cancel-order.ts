// src/billing/usecase/cancel-order.ts — Use case for cancelling orders

import { OrderRepository, OrderStatus } from "../domain/order";
import { DomainError, NotFoundError } from "../domain/errors";

export class CancelOrderUseCase {
  constructor(
    private readonly orderRepo: OrderRepository,
  ) {}

  async execute(orderId: string): Promise<void> {
    const order = await this.orderRepo.findById(orderId);
    if (!order) {
      throw new NotFoundError(`order ${orderId}`);
    }

    if (order.status.value !== OrderStatus.Pending.value) {
      throw new DomainError("order_not_pending", "only pending orders can be cancelled");
    }

    order.status = OrderStatus.Cancelled;
    await this.orderRepo.save(order);
  }
}

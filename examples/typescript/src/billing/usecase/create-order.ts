// src/billing/usecase/create-order.ts — Use case for creating orders

import { Order, OrderItem, OrderRepository, OrderStatus } from "../domain/order";
import { ValidationError } from "../../auth/domain/errors";
import { NotificationType } from "../../notifications/domain/notification";

export interface CreateOrderInput {
  userId: string;
  items: Array<{ productId: string; quantity: number }>;
}

export interface CreateOrderResult {
  order: Order;
}

export class CreateOrderUseCase {
  constructor(
    private readonly orderRepo: OrderRepository,
    private readonly notificationType: NotificationType,
  ) {}

  async execute(input: CreateOrderInput): Promise<CreateOrderResult> {
    if (!input.userId || input.userId.trim().length === 0) {
      throw new ValidationError("userId", "must not be empty");
    }

    if (!input.items || input.items.length === 0) {
      throw new ValidationError("items", "at least one item is required");
    }

    for (let i = 0; i < input.items.length; i++) {
      const item = input.items[i];
      if (!item.productId || item.productId.trim().length === 0) {
        throw new ValidationError(`items[${i}].productId`, "must not be empty");
      }
      if (item.quantity <= 0) {
        throw new ValidationError(`items[${i}].quantity`, "must be a positive number");
      }
    }

    const order: Order = {
      type: "order",
      id: `order-${Date.now()}`,
      userId: input.userId,
      items: input.items.map((item) => ({
        productId: item.productId,
        quantity: item.quantity,
        unitPriceCents: 0,
      })),
      status: OrderStatus.Pending,
      totalCents: 0,
      createdAt: new Date(),
      updatedAt: new Date(),
    };

    await this.orderRepo.save(order);

    return { order };
  }
}

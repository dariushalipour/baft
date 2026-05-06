// src/billing/domain/order.ts — Billing domain entities

export interface OrderStatus {
  readonly value: string;
}

function status(value: string): OrderStatus {
  return { value };
}

export const OrderStatus = {
  Pending: status("pending"),
  Confirmed: status("confirmed"),
  Shipped: status("shipped"),
  Delivered: status("delivered"),
  Cancelled: status("cancelled"),
} as const;

export interface OrderItem {
  productId: string;
  quantity: number;
  unitPriceCents: number;
}

export interface Order extends BaseEntity {
  type: "order";
  userId: string;
  items: OrderItem[];
  status: OrderStatus;
  totalCents: number;
  createdAt: Date;
  updatedAt: Date;
}

export interface BaseEntity {
  id: string;
  createdAt: Date;
  updatedAt: Date;
}

export interface OrderRepository {
  findById(id: string): Promise<Order | null>;
  save(order: Order): Promise<void>;
  listByUser(userId: string, limit: number): Promise<Order[]>;
}

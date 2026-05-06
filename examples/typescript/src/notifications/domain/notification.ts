// src/notifications/domain/notification.ts — Notification domain entities

export interface NotificationType {
  readonly value: string;
}

function notificationType(value: string): NotificationType {
  return { value };
}

export const NotificationType = {
  OrderConfirmed: notificationType("order_confirmed"),
  OrderCancelled: notificationType("order_cancelled"),
  PaymentReceived: notificationType("payment_received"),
} as const;

export interface Notification {
  id: string;
  userId: string;
  type: NotificationType;
  subject: string;
  body: string;
  sentAt: Date;
}

export interface NotificationRepository {
  save(notification: Notification): Promise<void>;
  findByUserId(userId: string): Promise<Notification[]>;
}

export interface NotificationChannel {
  send(to: string, subject: string, body: string): Promise<void>;
}

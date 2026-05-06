// src/notifications/usecase/send-notification.ts — Use case for sending notifications

import { Notification, NotificationRepository, NotificationChannel } from "../domain/notification";
import { ValidationError } from "../../auth/domain/errors";

export interface SendNotificationInput {
  userId: string;
  subject: string;
  body: string;
}

export class SendNotificationUseCase {
  constructor(
    private readonly repo: NotificationRepository,
    private readonly channel: NotificationChannel,
  ) {}

  async execute(input: SendNotificationInput): Promise<void> {
    if (!input.userId || input.userId.trim().length === 0) {
      throw new ValidationError("userId", "must not be empty");
    }
    if (!input.subject || input.subject.trim().length === 0) {
      throw new ValidationError("subject", "must not be empty");
    }
    if (!input.body || input.body.trim().length === 0) {
      throw new ValidationError("body", "must not be empty");
    }

    const notification: Notification = {
      id: `notif-${Date.now()}`,
      userId: input.userId,
      type: { value: "generic" },
      subject: input.subject,
      body: input.body,
      sentAt: new Date(),
    };

    await this.repo.save(notification);
    await this.channel.send(input.userId, input.subject, input.body);
  }
}

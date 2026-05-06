// src/billing/api/routes.ts — Billing HTTP route definitions

import { CreateOrderUseCase } from "../usecase/create-order";
import { CancelOrderUseCase } from "../usecase/cancel-order";
import { DomainError } from "../domain/errors";

interface Request {
  headers: Record<string, string | undefined>;
  method: string;
  path: string;
  body?: unknown;
}

interface Response {
  status(code: number): Response;
  json(body: unknown): void;
  send(body?: unknown): void;
}

export function registerRoutes(
  createOrder: CreateOrderUseCase,
  cancelOrder: CancelOrderUseCase,
) {
  return {
    async handleRequest(req: Request, res: Response): Promise<void> {
      if (req.method === "POST" && req.path === "/orders") {
        const input = req.body as { userId: string; items: Array<{ productId: string; quantity: number }> };
        const result = await createOrder.execute(input);
        res.status(201).json(result);
        return;
      }

      if (req.method === "POST" && req.path.match(/^\/orders\/[^/]+\/cancel$/)) {
        const orderId = req.path.split("/")[2];
        await cancelOrder.execute(orderId);
        res.status(204).send();
        return;
      }

      res.status(404).json({ error: "Not found" });
    },
  };
}

// src/api/routes.ts — HTTP route definitions

import { CreateOrderUseCase } from "../billing/usecase/create-order";
import { CancelOrderUseCase } from "../billing/usecase/cancel-order";
import { JwtService } from "../auth/usecase/jwt";
import { ConsoleLogger } from "../core/logger";
import { authMiddleware } from "../auth/usecase/middleware";
import { Logger } from "../core/logger";

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
  jwtService: JwtService,
  logger: Logger,
) {
  const auth = authMiddleware(jwtService);

  return {
    async handleRequest(req: Request, res: Response): Promise<void> {
      if (req.method === "POST" && req.path === "/orders") {
        await auth(req, res);
        const input = req.body as { userId: string; items: Array<{ productId: string; quantity: number }> };
        const result = await createOrder.execute(input);
        res.status(201).json(result);
        return;
      }

      if (req.method === "POST" && req.path.match(/^\/orders\/[^/]+\/cancel$/)) {
        await auth(req, res);
        const orderId = req.path.split("/")[2];
        await cancelOrder.execute(orderId);
        res.status(204).send();
        return;
      }

      res.status(404).json({ error: "Not found" });
    },
  };
}

export { ConsoleLogger };

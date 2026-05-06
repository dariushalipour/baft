// src/api/server.ts — HTTP server entry point

import { CreateOrderUseCase } from "../billing/usecase/create-order";
import { CancelOrderUseCase } from "../billing/usecase/cancel-order";
import { JwtService } from "../auth/usecase/jwt";
import { ConsoleLogger } from "../core/logger";
import { registerRoutes } from "./routes";
import { loadConfig } from "../core/config";

async function main() {
  const config = loadConfig();
  const logger = new ConsoleLogger();

  // In a real app, these would be wired up with DI
  const createOrder = new CreateOrderUseCase(
    {} as any,
  );
  const cancelOrder = new CancelOrderUseCase(
    {} as any,
  );
  const jwtService = new JwtService(
    {} as any,
    {} as any,
  );

  const router = registerRoutes(createOrder, cancelOrder, jwtService, logger);

  console.log(`server listening on port ${config.port}`);
}

main().catch(console.error);

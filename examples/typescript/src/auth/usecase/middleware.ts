// src/auth/usecase/middleware.ts — Authentication middleware

import { JwtService } from "./jwt";
import { DomainError } from "../domain/errors";

export interface Request {
  headers: Record<string, string | undefined>;
  path: string;
  user?: unknown;
}

export interface Response {
  status(code: number): Response;
  json(body: unknown): void;
}

export function authMiddleware(jwtService: JwtService) {
  return async (req: Request, res: Response): Promise<void> => {
    const token = req.headers["authorization"]?.replace("Bearer ", "");
    try {
      const user = await jwtService.requireAuth(token);
      req.user = user;
    } catch (err) {
      res.status(401).json({ error: "Unauthorized" });
      throw new DomainError("unauthorized", "Authentication failed");
    }
  };
}

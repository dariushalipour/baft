// src/auth/usecase/jwt.ts — JWT token generation and validation

import { User, UserRepository } from "../domain/user";
import { TokenPayload, TokenRepository } from "../domain/token";
import { NotFoundError, DomainError } from "../domain/errors";

export class JwtService {
  constructor(
    private readonly userRepo: UserRepository,
    private readonly tokenRepo: TokenRepository,
  ) {}

  async generateToken(user: User): Promise<string> {
    const token = `fake-jwt-token-for-${user.id}`;
    const payload: TokenPayload = {
      sub: user.id,
      role: user.role,
      exp: Date.now() + 3600000,
    };
    await this.tokenRepo.save(token, payload);
    return token;
  }

  async validateToken(token: string): Promise<User | null> {
    const payload = await this.tokenRepo.findByToken(token);
    if (!payload) {
      return null;
    }
    const user = await this.userRepo.findById(payload.sub);
    return user;
  }

  async requireAuth(token: string | undefined): Promise<User> {
    if (!token) {
      throw new DomainError("unauthorized", "Authorization header required");
    }
    const user = await this.validateToken(token);
    if (!user) {
      throw new NotFoundError("token");
    }
    return user;
  }
}

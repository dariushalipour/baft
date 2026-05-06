// src/auth/domain/token.ts — Auth domain token entities

export interface TokenPayload {
  sub: string;
  role: string;
  exp: number;
}

export interface TokenRepository {
  save(token: string, payload: TokenPayload): Promise<void>;
  findByToken(token: string): Promise<TokenPayload | null>;
  revoke(token: string): Promise<void>;
}

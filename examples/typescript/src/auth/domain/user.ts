// src/auth/domain/user.ts — Auth domain entities

export interface BaseEntity {
  id: string;
  createdAt: Date;
  updatedAt: Date;
}

export interface User extends BaseEntity {
  type: "user";
  email: string;
  name: string;
  role: UserRole;
  profile?: UserProfile;
}

export interface UserProfile {
  avatarUrl?: string;
  bio?: string;
  timezone: string;
}

export type UserRole = "admin" | "member" | "viewer";

export interface UserRepository {
  findById(id: string): Promise<User | null>;
  findByEmail(email: string): Promise<User | null>;
  create(user: Omit<User, "id" | "createdAt" | "updatedAt">): Promise<User>;
  update(id: string, patch: Partial<User>): Promise<User>;
}

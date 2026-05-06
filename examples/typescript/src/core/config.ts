// src/core/config.ts — Application configuration loader

export interface AppConfig {
  port: number;
  databaseUrl: string;
  jwtSecret: string;
  env: "development" | "staging" | "production";
}

let config: AppConfig | null = null;

export function loadConfig(env?: NodeJS.ProcessEnv): AppConfig {
  if (config) return config;
  const e = env || process.env;
  config = {
    port: parseInt(e.PORT || "3000", 10),
    databaseUrl: e.DATABASE_URL || "sqlite://:memory:",
    jwtSecret: e.JWT_SECRET || "dev-secret-change-me",
    env: (e.NODE_ENV as AppConfig["env"]) || "development",
  };
  return config;
}

export function resetConfig(): void {
  config = null;
}

// src/core/logger.ts — Structured logging utility

export interface LogEntry {
  level: "debug" | "info" | "warn" | "error";
  message: string;
  context: Record<string, unknown>;
  timestamp: Date;
}

export interface Logger {
  debug(message: string, context?: Record<string, unknown>): void;
  info(message: string, context?: Record<string, unknown>): void;
  warn(message: string, context?: Record<string, unknown>): void;
  error(message: string, context?: Record<string, unknown>): void;
}

export class ConsoleLogger implements Logger {
  debug(message: string, context?: Record<string, unknown>): void {
    this.write("debug", message, context);
  }
  info(message: string, context?: Record<string, unknown>): void {
    this.write("info", message, context);
  }
  warn(message: string, context?: Record<string, unknown>): void {
    this.write("warn", message, context);
  }
  error(message: string, context?: Record<string, unknown>): void {
    this.write("error", message, context);
  }

  private write(level: string, message: string, context?: Record<string, unknown>): void {
    const entry: LogEntry = {
      level: level as LogEntry["level"],
      message,
      context: context || {},
      timestamp: new Date(),
    };
    const line = JSON.stringify(entry);
    if (entry.level === "error") {
      console.error(line);
    } else if (entry.level === "warn") {
      console.warn(line);
    } else {
      console.log(line);
    }
  }
}

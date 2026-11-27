import { mkdirSync } from "node:fs";
import { dirname } from "node:path";
import { DatabaseSync } from "node:sqlite";

// The SQLite file shared with the Go backend. Better Auth owns its `user`,
// `session`, `account` and `verification` tables here; Go owns every other
// table (ADR-0004).
//
// `next build` loads this module without a database configured, so the build
// gets a throwaway in-memory database instead.
function openDatabase(): DatabaseSync {
  const path = process.env.DATABASE_PATH;
  if (!path) {
    if (process.env.NEXT_PHASE === "phase-production-build") {
      return new DatabaseSync(":memory:");
    }
    throw new Error("DATABASE_PATH is not set (see .env.example)");
  }

  mkdirSync(dirname(path), { recursive: true });
  const db = new DatabaseSync(path);
  // WAL lets Go and Next.js read and write the same file; the busy timeout
  // makes a writer wait for the other process's lock instead of failing.
  db.exec("PRAGMA journal_mode = WAL");
  db.exec("PRAGMA busy_timeout = 5000");
  db.exec("PRAGMA foreign_keys = ON");
  return db;
}

// Reuse one connection across hot reloads in development.
const globalForDb = globalThis as unknown as { squadsyncDb?: DatabaseSync };

export const db = globalForDb.squadsyncDb ?? openDatabase();
globalForDb.squadsyncDb = db;

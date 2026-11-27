// Client for the Go backend's internal API (ADR-0004). Only server code may
// import this: it carries the shared secret, and the browser never talks to
// Go. (It doesn't import "server-only" because the Better Auth CLI loads it
// through src/lib/auth.ts.)
//
// The types here are kept in sync by hand with
// squadsync-be/internal/httpapi/httpapi.go.

const HEADER_SECRET = "X-Internal-Secret";
const HEADER_FELHASZNALO_ID = "X-Felhasznalo-Id";

export type Felhasznalo = {
  id: string;
  name: string;
  /** RFC 3339, UTC */
  createdAt: string;
};

export class BackendError extends Error {
  constructor(
    readonly status: number,
    message: string,
  ) {
    super(message);
    this.name = "BackendError";
  }
}

function backendUrl(path: string): string {
  const port = process.env.BACKEND_PORT || "8081";
  return `http://127.0.0.1:${port}${path}`;
}

async function callBackend<T>(
  felhasznaloId: string,
  method: "GET" | "POST",
  path: string,
  body?: unknown,
): Promise<T> {
  const secret = process.env.INTERNAL_API_SECRET;
  if (!secret) {
    throw new Error("INTERNAL_API_SECRET is not set (see .env.example)");
  }

  const res = await fetch(backendUrl(path), {
    method,
    headers: {
      [HEADER_SECRET]: secret,
      [HEADER_FELHASZNALO_ID]: felhasznaloId,
      ...(body === undefined ? {} : { "Content-Type": "application/json" }),
    },
    body: body === undefined ? undefined : JSON.stringify(body),
    cache: "no-store",
    // Go is on localhost; if it hangs, fail fast instead of hanging the page.
    signal: AbortSignal.timeout(5000),
  });

  if (!res.ok) {
    const text = await res.text().catch(() => "");
    throw new BackendError(
      res.status,
      `${method} ${path} failed with ${res.status}: ${text}`,
    );
  }
  return (await res.json()) as T;
}

/**
 * Tells Go about a Felhasználó's first login. Go creates the Felhasználó
 * with the same id as Better Auth's user.id and records their main Discord
 * account. Idempotent: calling it again returns the existing Felhasználó.
 */
export function firstLogin(
  felhasznaloId: string,
  input: { discordId: string; name: string },
): Promise<Felhasznalo> {
  return callBackend(felhasznaloId, "POST", "/felhasznalo/first-login", input);
}

/** Returns the Felhasználó, or null if Go doesn't know them (yet). */
export async function getFelhasznalo(
  felhasznaloId: string,
): Promise<Felhasznalo | null> {
  try {
    return await callBackend<Felhasznalo>(felhasznaloId, "GET", "/felhasznalo");
  } catch (err) {
    if (err instanceof BackendError && err.status === 404) {
      return null;
    }
    throw err;
  }
}

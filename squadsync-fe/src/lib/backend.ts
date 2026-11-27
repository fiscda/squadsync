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

/**
 * Calls Go. Pass the Felhasználó's id for routes that act for them, or null
 * for the few routes that run before a Felhasználó exists.
 */
async function callBackend<T>(
  felhasznaloId: string | null,
  method: "GET" | "POST" | "DELETE",
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
      ...(felhasznaloId === null
        ? {}
        : { [HEADER_FELHASZNALO_ID]: felhasznaloId }),
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
  if (res.status === 204) {
    return undefined as T;
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

// --- Csoport and Csoport-meghívó ---

export type Csoport = {
  id: string;
  name: string;
  /** RFC 3339, UTC */
  createdAt: string;
};

/** A Csoport in the list of the Csoports a Felhasználó is a Tag of. */
export type CsoportSummary = Csoport & {
  /** Whether the Felhasználó is an Admin of it. */
  admin: boolean;
  tagCount: number;
};

export type Tag = {
  felhasznaloId: string;
  name: string;
  admin: boolean;
  /** RFC 3339, UTC */
  createdAt: string;
};

export type CsoportDetail = Csoport & {
  /** Whether the caller is an Admin of the Csoport. */
  admin: boolean;
  /** Admins first, then in the order they joined. */
  tags: Tag[];
};

export type CsoportMeghivo = {
  id: string;
  /** The unguessable secret in the invite link. */
  token: string;
  /** RFC 3339, UTC */
  createdAt: string;
};

export type JoinResult = Csoport & {
  /** False when the Felhasználó already was a Tag, so nothing changed. */
  joined: boolean;
};

export function createCsoport(
  felhasznaloId: string,
  name: string,
): Promise<Csoport> {
  return callBackend(felhasznaloId, "POST", "/csoport", { name });
}

export function listCsoportok(
  felhasznaloId: string,
): Promise<CsoportSummary[]> {
  return callBackend(felhasznaloId, "GET", "/csoport");
}

/** Returns null when the Csoport doesn't exist or the caller isn't a Tag. */
export async function getCsoport(
  felhasznaloId: string,
  csoportId: string,
): Promise<CsoportDetail | null> {
  return nullOnNotFound(
    callBackend(
      felhasznaloId,
      "GET",
      `/csoport/${encodeURIComponent(csoportId)}`,
    ),
  );
}

export function createCsoportMeghivo(
  felhasznaloId: string,
  csoportId: string,
): Promise<CsoportMeghivo> {
  return callBackend(
    felhasznaloId,
    "POST",
    `/csoport/${encodeURIComponent(csoportId)}/meghivo`,
  );
}

/** The Csoport-meghívók that still work. Null when the caller isn't a Tag. */
export function listCsoportMeghivok(
  felhasznaloId: string,
  csoportId: string,
): Promise<CsoportMeghivo[] | null> {
  return nullOnNotFound(
    callBackend(
      felhasznaloId,
      "GET",
      `/csoport/${encodeURIComponent(csoportId)}/meghivo`,
    ),
  );
}

export async function revokeCsoportMeghivo(
  felhasznaloId: string,
  csoportId: string,
  meghivoId: string,
): Promise<void> {
  await callBackend<void>(
    felhasznaloId,
    "DELETE",
    `/csoport/${encodeURIComponent(csoportId)}/meghivo/${encodeURIComponent(meghivoId)}`,
  );
}

export type MeghivoPreview =
  | { status: "ok"; csoport: Csoport }
  | { status: "revoked" }
  | { status: "not-found" };

/** Which Csoport a link is for. Needs no login: the token is the secret. */
export async function previewCsoportMeghivo(
  token: string,
): Promise<MeghivoPreview> {
  try {
    const csoport = await callBackend<Csoport>(
      null,
      "GET",
      `/csoport-meghivo/${encodeURIComponent(token)}`,
    );
    return { status: "ok", csoport };
  } catch (err) {
    return meghivoRefusal(err);
  }
}

export type JoinOutcome =
  | { status: "ok"; result: JoinResult }
  | { status: "revoked" }
  | { status: "not-found" };

export async function joinCsoport(
  felhasznaloId: string,
  token: string,
): Promise<JoinOutcome> {
  try {
    const result = await callBackend<JoinResult>(
      felhasznaloId,
      "POST",
      `/csoport-meghivo/${encodeURIComponent(token)}/join`,
    );
    return { status: "ok", result };
  } catch (err) {
    return meghivoRefusal(err);
  }
}

/** Go answers 410 for a revoked link and 404 for an unknown one. */
function meghivoRefusal(err: unknown): {
  status: "revoked" | "not-found";
} {
  if (err instanceof BackendError) {
    if (err.status === 410) {
      return { status: "revoked" };
    }
    if (err.status === 404) {
      return { status: "not-found" };
    }
  }
  throw err;
}

async function nullOnNotFound<T>(call: Promise<T>): Promise<T | null> {
  try {
    return await call;
  } catch (err) {
    if (err instanceof BackendError && err.status === 404) {
      return null;
    }
    throw err;
  }
}

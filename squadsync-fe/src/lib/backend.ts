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
  method: "GET" | "POST" | "PUT" | "DELETE",
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

/** All that a link's holder may learn before joining: the Csoport's name. */
export type MeghivoCsoport = { name: string };

export type MeghivoPreview =
  | { status: "ok"; csoport: MeghivoCsoport }
  | { status: "revoked" }
  | { status: "not-found" };

/** Which Csoport a link is for. Needs no login: the token is the secret. */
export async function previewCsoportMeghivo(
  token: string,
): Promise<MeghivoPreview> {
  try {
    const csoport = await callBackend<MeghivoCsoport>(
      null,
      "GET",
      `/csoport-meghivo/${encodeURIComponent(token)}`,
    );
    return { status: "ok", csoport };
  } catch (err) {
    return meghivoRefusal(err);
  }
}

/**
 * The id of the Csoport behind a link if the Felhasználó already is a Tag of
 * it, even when the link was revoked. Null for anyone else.
 */
export async function getTagCsoportOfMeghivo(
  felhasznaloId: string,
  token: string,
): Promise<string | null> {
  const found = await nullOnNotFound(
    callBackend<{ csoportId: string }>(
      felhasznaloId,
      "GET",
      `/csoport-meghivo/${encodeURIComponent(token)}/tag`,
    ),
  );
  return found?.csoportId ?? null;
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

/**
 * Go answers 410 for a revoked link and 404 for an unknown one. (An unknown
 * Felhasználó is a 403, which stays an error.)
 */
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

// --- Esemény, Időpont and Visszajelzés ---

/** Egyeztetés → Véglegesítve → Lezajlott, or Elmaradt. */
export type Allapot = "egyeztetes" | "veglegesitve" | "elmaradt" | "lezajlott";

export type Valasz = "igen" | "talan" | "nem";

export type Helyszin = {
  /** "" when none. */
  name: string;
  /** An absolute http(s) URL, meant for Google Maps; "" when none. */
  url: string;
};

export type Esemeny = {
  id: string;
  csoportId: string;
  title: string;
  /** "" when none. */
  description: string;
  helyszin: Helyszin;
  szervezoId: string;
  allapot: Allapot;
  /** RFC 3339, UTC */
  createdAt: string;
};

export type EsemenySummary = Esemeny & {
  idopontCount: number;
  /** The earliest Időpont's start (RFC 3339, UTC), null if none. */
  firstStart: string | null;
};

export type VisszajelzesOfEsemeny = {
  felhasznaloId: string;
  name: string;
  valasz: Valasz;
  /** False for someone who is no longer a Tag: they don't count. */
  isTag: boolean;
  /** RFC 3339, UTC */
  updatedAt: string;
};

export type Idopont = {
  id: string;
  /** RFC 3339, UTC */
  start: string;
  /** RFC 3339, UTC; null when it has only a start. */
  end: string | null;
  /**
   * A whole-day Időpont: start is Budapest midnight of the first day and end
   * Budapest midnight after the last day. Show dates only.
   */
  egeszNapos: boolean;
  addedBy: string;
  /** Counts of Tags' Visszajelzések. */
  igen: number;
  talan: number;
  nem: number;
  /** 1 is first; null when no Tag has answered. */
  helyezes: number | null;
  /** The caller's own answer, null if none. */
  sajat: Valasz | null;
  /** Ordered by name. */
  visszajelzesek: VisszajelzesOfEsemeny[];
};

export type EsemenyDetail = Esemeny & {
  csoportName: string;
  szervezoName: string;
  /** In chronological order. */
  idopontok: Idopont[];
};

export type EsemenyInput = {
  title: string;
  description: string;
  helyszin: Helyszin;
  /**
   * Budapest wall clock times like "2026-12-05T20:00"; end may be "". With
   * egeszNapos, the first and last day like "2026-12-05" (both included).
   */
  idopontok: { start: string; end: string; egeszNapos: boolean }[];
};

export function proposeEsemeny(
  felhasznaloId: string,
  csoportId: string,
  input: EsemenyInput,
): Promise<Esemeny> {
  return callBackend(
    felhasznaloId,
    "POST",
    `/csoport/${encodeURIComponent(csoportId)}/esemeny`,
    input,
  );
}

/** Newest first. Null when the caller isn't a Tag. */
export function listEsemenyek(
  felhasznaloId: string,
  csoportId: string,
): Promise<EsemenySummary[] | null> {
  return nullOnNotFound(
    callBackend(
      felhasznaloId,
      "GET",
      `/csoport/${encodeURIComponent(csoportId)}/esemeny`,
    ),
  );
}

/** Null when the Esemény doesn't exist or the caller can't see it. */
export function getEsemeny(
  felhasznaloId: string,
  esemenyId: string,
): Promise<EsemenyDetail | null> {
  return nullOnNotFound(
    callBackend(
      felhasznaloId,
      "GET",
      `/esemeny/${encodeURIComponent(esemenyId)}`,
    ),
  );
}

/** Gives or changes the caller's Visszajelzés on one Időpont. */
export async function setVisszajelzes(
  felhasznaloId: string,
  esemenyId: string,
  idopontId: string,
  valasz: Valasz,
): Promise<void> {
  await callBackend<void>(
    felhasznaloId,
    "PUT",
    `/esemeny/${encodeURIComponent(esemenyId)}/idopont/${encodeURIComponent(idopontId)}/visszajelzes`,
    { valasz },
  );
}

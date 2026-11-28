import "server-only";
import { cache } from "react";
import {
  BackendError,
  callBackend,
  nullOnNotFound,
  type Valasz,
  type Vendeg,
} from "./backend";
import {
  ensureFelhasznalo,
  getCurrentFelhasznalo,
  requireFelhasznalo,
} from "./felhasznalo";

// Vendégek (#13): Go's Esemény-meghívó and Vendég routes. The types are kept
// in sync by hand with squadsync-be/internal/httpapi/vendeg.go. Reads are
// side-effect free and cached for one request; the rest are mutations, for
// server actions and route handlers only.

/** A working Esemény-meghívó link. */
export type EsemenyMeghivo = {
  id: string;
  token: string;
  /** RFC 3339, UTC */
  createdAt: string;
};

/** All that a link's holder may learn before joining. */
export type EsemenyMeghivoEsemeny = { title: string; szervezoName: string };

export type EsemenyMeghivoPreview =
  | { status: "ok"; esemeny: EsemenyMeghivoEsemeny }
  /** Revoked by a Tag. */
  | { status: "revoked" }
  /** The Esemény is Lezajlott or Elmaradt. */
  | { status: "expired" }
  | { status: "not-found" };

type Refusal = { status: "revoked" | "expired" | "not-found" };

/**
 * Go answers 410 with code "revoked" or "expired" for a link that no longer
 * works, and 404 for an unknown one. Anything else stays an error.
 */
function refusal(err: unknown): Refusal {
  if (err instanceof BackendError) {
    if (err.status === 410) {
      return { status: err.code === "expired" ? "expired" : "revoked" };
    }
    if (err.status === 404) {
      return { status: "not-found" };
    }
  }
  throw err;
}

function tokenPath(token: string): string {
  return `/esemeny-meghivo/${encodeURIComponent(token)}`;
}

function esemenyPath(esemenyId: string): string {
  return `/esemeny/${encodeURIComponent(esemenyId)}`;
}

/** Which Esemény a link is for, or why it doesn't work. Needs no login. */
export const getEsemenyMeghivoPreview = cache(
  async (token: string): Promise<EsemenyMeghivoPreview> => {
    try {
      const esemeny = await callBackend<EsemenyMeghivoEsemeny>(
        null,
        "GET",
        tokenPath(token),
      );
      return { status: "ok", esemeny };
    } catch (err) {
      return refusal(err);
    }
  },
);

/**
 * The Esemény behind a link if the logged-in Felhasználó already takes part
 * in it (a Tag or a Vendég), even when the link no longer works, so they
 * can be sent straight in. Null for anyone else and when logged out.
 */
export const getMemberEsemenyOfMeghivo = cache(
  async (token: string): Promise<string | null> => {
    const current = await getCurrentFelhasznalo();
    if (current.status !== "ok") {
      return null;
    }
    const found = await nullOnNotFound(
      callBackend<{ esemenyId: string }>(
        current.felhasznalo.id,
        "GET",
        `${tokenPath(token)}/esemeny`,
      ),
    );
    return found?.esemenyId ?? null;
  },
);

/** The working Esemény-meghívók of an Esemény, or null for a non-Tag. */
export const getEsemenyMeghivok = cache(
  async (esemenyId: string): Promise<EsemenyMeghivo[] | null> => {
    const felhasznalo = await requireFelhasznalo();
    return nullOnNotFound(
      callBackend<EsemenyMeghivo[]>(
        felhasznalo.id,
        "GET",
        `${esemenyPath(esemenyId)}/meghivo`,
      ),
    );
  },
);

export type JoinEsemenyResult =
  | { status: "ok"; esemenyId: string; joined: boolean }
  | { status: "anonymous" }
  | Refusal;

/**
 * Makes the logged-in Felhasználó a Vendég of the Esemény behind a link. A
 * Tag, or someone who already is a Vendég, just gets the Esemény back.
 *
 * It's a mutation, so call it only from a route handler or a server action,
 * never while rendering.
 */
export async function joinEsemenyWithToken(
  token: string,
): Promise<JoinEsemenyResult> {
  const felhasznaloId = await ensureFelhasznalo();
  if (felhasznaloId === null) {
    return { status: "anonymous" };
  }
  try {
    const result = await callBackend<{ esemenyId: string; joined: boolean }>(
      felhasznaloId,
      "POST",
      `${tokenPath(token)}/join`,
    );
    return { status: "ok", ...result };
  } catch (err) {
    return refusal(err);
  }
}

export async function createEsemenyMeghivo(
  felhasznaloId: string,
  esemenyId: string,
): Promise<EsemenyMeghivo> {
  return callBackend(
    felhasznaloId,
    "POST",
    `${esemenyPath(esemenyId)}/meghivo`,
  );
}

export async function revokeEsemenyMeghivo(
  felhasznaloId: string,
  esemenyId: string,
  meghivoId: string,
): Promise<void> {
  await callBackend<void>(
    felhasznaloId,
    "DELETE",
    `${esemenyPath(esemenyId)}/meghivo/${encodeURIComponent(meghivoId)}`,
  );
}

/** Adds a name-only Vendég; the caller then answers and claims for them. */
export async function addVendeg(
  felhasznaloId: string,
  esemenyId: string,
  name: string,
): Promise<Vendeg> {
  return callBackend(
    felhasznaloId,
    "POST",
    `${esemenyPath(esemenyId)}/vendeg`,
    {
      name,
    },
  );
}

/**
 * Removes a Vendég. Go answers 403 for someone who may not, and 409 with
 * code "vendeg_has_kiadas" for a Vendég who is in a Kiadás.
 */
export async function removeVendeg(
  felhasznaloId: string,
  esemenyId: string,
  vendegId: string,
): Promise<void> {
  await callBackend<void>(
    felhasznaloId,
    "DELETE",
    `${esemenyPath(esemenyId)}/vendeg/${encodeURIComponent(vendegId)}`,
  );
}

/** The Tag who added a name-only Vendég answers one Időpont for them. */
export async function setVendegVisszajelzes(
  felhasznaloId: string,
  esemenyId: string,
  idopontId: string,
  vendegId: string,
  valasz: Valasz,
): Promise<void> {
  await callBackend<void>(
    felhasznaloId,
    "PUT",
    `${esemenyPath(esemenyId)}/idopont/${encodeURIComponent(idopontId)}/vendeg/${encodeURIComponent(vendegId)}/visszajelzes`,
    { valasz },
  );
}

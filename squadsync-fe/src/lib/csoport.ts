import "server-only";
import { cache } from "react";
import {
  getCsoport as backendGetCsoport,
  type CsoportDetail,
  type CsoportMeghivo,
  type CsoportSummary,
  joinCsoport,
  listCsoportMeghivok,
  listCsoportok,
  type MeghivoPreview,
  previewCsoportMeghivo,
} from "./backend";
import { ensureFelhasznalo, requireFelhasznalo } from "./felhasznalo";

// Reads are side-effect free and cached for one request, so a page and its
// components can ask for the same thing without calling Go twice.

/** The Csoports of the logged-in Felhasználó. Redirects home if logged out. */
export const getMyCsoportok = cache(async (): Promise<CsoportSummary[]> => {
  const felhasznalo = await requireFelhasznalo();
  return listCsoportok(felhasznalo.id);
});

/** A Csoport with its Tags, or null if the caller isn't a Tag of it. */
export const getCsoport = cache(
  async (csoportId: string): Promise<CsoportDetail | null> => {
    const felhasznalo = await requireFelhasznalo();
    return backendGetCsoport(felhasznalo.id, csoportId);
  },
);

/** The working Csoport-meghívók of a Csoport, or null for a non-Tag. */
export const getCsoportMeghivok = cache(
  async (csoportId: string): Promise<CsoportMeghivo[] | null> => {
    const felhasznalo = await requireFelhasznalo();
    return listCsoportMeghivok(felhasznalo.id, csoportId);
  },
);

/** Which Csoport a link is for, or why it doesn't work. Needs no login. */
export const getMeghivoPreview = cache(
  (token: string): Promise<MeghivoPreview> => previewCsoportMeghivo(token),
);

export type JoinWithTokenResult =
  | { status: "joined" | "already-tag"; csoportId: string }
  | { status: "revoked" | "not-found" | "anonymous" };

/**
 * Makes the logged-in Felhasználó a Tag of the Csoport behind a link token.
 * Joining again as a Tag changes nothing.
 *
 * It's a mutation, so call it only from a route handler or a server action,
 * never while rendering.
 */
export async function joinWithToken(
  token: string,
): Promise<JoinWithTokenResult> {
  const felhasznaloId = await ensureFelhasznalo();
  if (felhasznaloId === null) {
    return { status: "anonymous" };
  }
  const outcome = await joinCsoport(felhasznaloId, token);
  if (outcome.status !== "ok") {
    return { status: outcome.status };
  }
  return {
    status: outcome.result.joined ? "joined" : "already-tag",
    csoportId: outcome.result.id,
  };
}

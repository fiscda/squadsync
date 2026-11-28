import "server-only";
import { cache } from "react";
import { BackendError, callBackend } from "../backend";
import { requireFelhasznalo } from "../felhasznalo";

// Felszerelés: Go's /esemeny/{id}/felszereles routes. The types are kept in
// sync by hand with squadsync-be/internal/httpapi/felszereles.go.

export type Tetel = {
  id: string;
  name: string;
  /** How many are needed, at least 1. */
  quantity: number;
  /** A reminder for everyone, never claimed. */
  mindenkiHozza: boolean;
  addedBy: string;
  /** RFC 3339, UTC */
  createdAt: string;
};

/** Part of a Tétel claimed by someone. */
export type Vallalas = {
  felhasznaloId: string;
  /** A name-only Vendég's id, "" otherwise. */
  vendegId: string;
  name: string;
  quantity: number;
};

export type TetelAllas = Tetel & {
  vallalt: number;
  /** Still missing (0 for a "mindenki hozza" Tétel). */
  hianyzik: number;
  /** Ordered by name. */
  vallalasok: Vallalas[];
  /** How many the caller claims. */
  sajat: number;
  /** Whether the caller may edit or delete it now. */
  canEdit: boolean;
};

export type Felszereles = {
  /** Oldest first. */
  tetelek: TetelAllas[];
  /** Missing pieces over every claimable Tétel. */
  hianyzik: number;
  /** The "mindenki hozza" Tételek, set while the caller is a Résztvevő. */
  emlekezteto: string[];
  canAdd: boolean;
  canClaim: boolean;
};

export type TetelInput = {
  name: string;
  quantity: number;
  mindenkiHozza: boolean;
};

function base(esemenyId: string): string {
  return `/esemeny/${encodeURIComponent(esemenyId)}/felszereles`;
}

/**
 * The Esemény's Felszerelés list, or null if the caller can't see it or it
 * is switched off.
 */
export const getFelszereles = cache(
  async (esemenyId: string): Promise<Felszereles | null> => {
    const felhasznalo = await requireFelhasznalo();
    try {
      return await callBackend<Felszereles>(
        felhasznalo.id,
        "GET",
        base(esemenyId),
      );
    } catch (err) {
      if (
        err instanceof BackendError &&
        (err.status === 404 || err.status === 409)
      ) {
        return null;
      }
      throw err;
    }
  },
);

export async function addTetel(
  felhasznaloId: string,
  esemenyId: string,
  input: TetelInput,
): Promise<void> {
  await callBackend(felhasznaloId, "POST", `${base(esemenyId)}/tetel`, input);
}

export async function updateTetel(
  felhasznaloId: string,
  esemenyId: string,
  tetelId: string,
  input: TetelInput,
): Promise<void> {
  await callBackend<void>(
    felhasznaloId,
    "PUT",
    `${base(esemenyId)}/tetel/${encodeURIComponent(tetelId)}`,
    input,
  );
}

export async function deleteTetel(
  felhasznaloId: string,
  esemenyId: string,
  tetelId: string,
): Promise<void> {
  await callBackend<void>(
    felhasznaloId,
    "DELETE",
    `${base(esemenyId)}/tetel/${encodeURIComponent(tetelId)}`,
  );
}

/**
 * Sets how many of the Tétel the caller (or, with vendegId, their name-only
 * Vendég) claims; 0 removes the claim.
 */
export async function setVallalas(
  felhasznaloId: string,
  esemenyId: string,
  tetelId: string,
  quantity: number,
  vendegId = "",
): Promise<void> {
  await callBackend<void>(
    felhasznaloId,
    "PUT",
    `${base(esemenyId)}/tetel/${encodeURIComponent(tetelId)}/vallalas`,
    { quantity, vendegId },
  );
}

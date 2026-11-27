import "server-only";
import { cache } from "react";
import { BackendError, callBackend } from "../backend";
import { requireFelhasznalo } from "../felhasznalo";

// Autók: Go's /esemeny/{id}/autok routes. The types are kept in sync by hand
// with squadsync-be/internal/httpapi/autok.go.

export type Auto = {
  id: string;
  soforId: string;
  soforName: string;
  /** Free seats, not counting the Sofőr (1 to 8). */
  seats: number;
  note: string;
  /** RFC 3339, UTC */
  createdAt: string;
};

/** Someone in an Autó, saját úton, or without a seat. */
export type Szemely = {
  felhasznaloId: string;
  /** A name-only Vendég's id (#13), "" otherwise. */
  vendegId?: string;
  name: string;
};

export type AutoAllas = Auto & {
  /** Ordered by name. */
  utasok: Szemely[];
  szabad: number;
  /** The caller is its Sofőr. */
  canEdit: boolean;
  /** The caller is its Sofőr or the Szervező. */
  canDelete: boolean;
};

export type Autok = {
  /** Oldest first. */
  autok: AutoAllas[];
  sajatUton: Szemely[];
  /** Résztvevők with no seat who aren't saját úton. */
  helyNelkul: Szemely[];
  canOffer: boolean;
  canClaim: boolean;
  /** The Autó the caller drives, "" for none. */
  sajatAutoId: string;
  /** The Autó the caller has a seat in, "" for none. */
  sajatUlesAutoId: string;
  sajatUtonVan: boolean;
  isSzervezo: boolean;
};

/** An Autó's Sofőr and passengers (for the fuel shortcut, #21). */
export type AutoUtasai = {
  auto: Auto;
  utasok: Szemely[];
};

export type AutoInput = {
  seats: number;
  note: string;
};

function base(esemenyId: string): string {
  return `/esemeny/${encodeURIComponent(esemenyId)}/autok`;
}

function autoPath(esemenyId: string, autoId: string): string {
  return `${base(esemenyId)}/auto/${encodeURIComponent(autoId)}`;
}

/**
 * The Esemény's Autók, or null if the caller can't see them or Autók is
 * switched off.
 */
export const getAutok = cache(
  async (esemenyId: string): Promise<Autok | null> => {
    const felhasznalo = await requireFelhasznalo();
    try {
      return await callBackend<Autok>(felhasznalo.id, "GET", base(esemenyId));
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

/** An Autó's Sofőr and passengers, for #21's fuel shortcut. */
export async function getAutoUtasai(
  felhasznaloId: string,
  esemenyId: string,
  autoId: string,
): Promise<AutoUtasai> {
  return await callBackend<AutoUtasai>(
    felhasznaloId,
    "GET",
    `${autoPath(esemenyId, autoId)}/utasai`,
  );
}

export async function offerAuto(
  felhasznaloId: string,
  esemenyId: string,
  input: AutoInput,
): Promise<void> {
  await callBackend(felhasznaloId, "POST", `${base(esemenyId)}/auto`, input);
}

export async function updateAuto(
  felhasznaloId: string,
  esemenyId: string,
  autoId: string,
  input: AutoInput,
): Promise<void> {
  await callBackend<void>(
    felhasznaloId,
    "PUT",
    autoPath(esemenyId, autoId),
    input,
  );
}

export async function deleteAuto(
  felhasznaloId: string,
  esemenyId: string,
  autoId: string,
): Promise<void> {
  await callBackend<void>(felhasznaloId, "DELETE", autoPath(esemenyId, autoId));
}

/** The caller claims a seat in the Autó. */
export async function claimUles(
  felhasznaloId: string,
  esemenyId: string,
  autoId: string,
): Promise<void> {
  await callBackend<void>(
    felhasznaloId,
    "POST",
    `${autoPath(esemenyId, autoId)}/ules`,
    {},
  );
}

/**
 * Takes a seat away: the caller's, or (the Szervező only) another
 * Felhasználó's.
 */
export async function releaseUles(
  felhasznaloId: string,
  esemenyId: string,
  targetId?: string,
): Promise<void> {
  const query = targetId
    ? `?felhasznaloId=${encodeURIComponent(targetId)}`
    : "";
  await callBackend<void>(
    felhasznaloId,
    "DELETE",
    `${base(esemenyId)}/ules${query}`,
  );
}

/** The Szervező puts someone into an Autó, out of their old one. */
export async function moveUtas(
  felhasznaloId: string,
  esemenyId: string,
  autoId: string,
  targetId: string,
): Promise<void> {
  await callBackend<void>(
    felhasznaloId,
    "PUT",
    `${autoPath(esemenyId, autoId)}/utas`,
    { felhasznaloId: targetId },
  );
}

export async function setSajatUton(
  felhasznaloId: string,
  esemenyId: string,
  on: boolean,
): Promise<void> {
  await callBackend<void>(
    felhasznaloId,
    "PUT",
    `${base(esemenyId)}/sajat-uton`,
    { on },
  );
}

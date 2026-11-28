import "server-only";
import { cache } from "react";
import { BackendError, callBackend } from "../backend";
import { requireFelhasznalo } from "../felhasznalo";

// Szállás: Go's /esemeny/{id}/szallas routes. The types are kept in sync by
// hand with squadsync-be/internal/httpapi/szallas.go.

/** Someone sleeping in an Ágy, marked máshol alszik, or without a bed. */
export type Szemely = {
  felhasznaloId: string;
  /** A name-only Vendég's id, "" or absent otherwise. */
  vendegId?: string;
  name: string;
};

export type AgyAllas = {
  id: string;
  szobaId: string;
  /** Férőhely: 1 (single) or 2 (double). */
  capacity: number;
  label: string;
  /** Ordered by name. */
  alvok: Szemely[];
  szabad: number;
};

export type SzobaAllas = {
  id: string;
  name: string;
  agyak: AgyAllas[];
};

export type Szallas = {
  note: string;
  /** In order. */
  szobak: SzobaAllas[];
  masholAlszik: Szemely[];
  /** Résztvevők with no bed who aren't máshol alszik. */
  agyNelkul: Szemely[];
  /** The caller, the Szervező, may set it up now. */
  canSetup: boolean;
  /** May take a spot or go máshol alszik: a Résztvevő in Véglegesítve. */
  canClaim: boolean;
  /** The Ágy the caller sleeps in, "" for none. */
  sajatAgyId: string;
  sajatMasholAlszik: boolean;
  isSzervezo: boolean;
};

export type AgyInput = {
  capacity: number;
  label: string;
};

function base(esemenyId: string): string {
  return `/esemeny/${encodeURIComponent(esemenyId)}/szallas`;
}

function szobaPath(esemenyId: string, szobaId: string): string {
  return `${base(esemenyId)}/szoba/${encodeURIComponent(szobaId)}`;
}

function agyPath(esemenyId: string, agyId: string): string {
  return `${base(esemenyId)}/agy/${encodeURIComponent(agyId)}`;
}

/**
 * The Esemény's Szállás, or null if the caller can't see it or Szállás is
 * switched off.
 */
export const getSzallas = cache(
  async (esemenyId: string): Promise<Szallas | null> => {
    const felhasznalo = await requireFelhasznalo();
    try {
      return await callBackend<Szallas>(felhasznalo.id, "GET", base(esemenyId));
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

export async function setSzallasNote(
  felhasznaloId: string,
  esemenyId: string,
  note: string,
): Promise<void> {
  await callBackend<void>(felhasznaloId, "PUT", base(esemenyId), { note });
}

export async function addSzoba(
  felhasznaloId: string,
  esemenyId: string,
  name: string,
): Promise<void> {
  await callBackend(felhasznaloId, "POST", `${base(esemenyId)}/szoba`, {
    name,
  });
}

export async function renameSzoba(
  felhasznaloId: string,
  esemenyId: string,
  szobaId: string,
  name: string,
): Promise<void> {
  await callBackend<void>(felhasznaloId, "PUT", szobaPath(esemenyId, szobaId), {
    name,
  });
}

export async function deleteSzoba(
  felhasznaloId: string,
  esemenyId: string,
  szobaId: string,
): Promise<void> {
  await callBackend<void>(
    felhasznaloId,
    "DELETE",
    szobaPath(esemenyId, szobaId),
  );
}

export async function addAgy(
  felhasznaloId: string,
  esemenyId: string,
  szobaId: string,
  input: AgyInput,
): Promise<void> {
  await callBackend(
    felhasznaloId,
    "POST",
    `${szobaPath(esemenyId, szobaId)}/agy`,
    input,
  );
}

export async function updateAgy(
  felhasznaloId: string,
  esemenyId: string,
  agyId: string,
  input: AgyInput,
): Promise<void> {
  await callBackend<void>(
    felhasznaloId,
    "PUT",
    agyPath(esemenyId, agyId),
    input,
  );
}

export async function deleteAgy(
  felhasznaloId: string,
  esemenyId: string,
  agyId: string,
): Promise<void> {
  await callBackend<void>(felhasznaloId, "DELETE", agyPath(esemenyId, agyId));
}

/** The caller takes a spot in the Ágy, or one for their name-only Vendég. */
export async function claimAgy(
  felhasznaloId: string,
  esemenyId: string,
  agyId: string,
  vendegId = "",
): Promise<void> {
  await callBackend<void>(
    felhasznaloId,
    "POST",
    `${agyPath(esemenyId, agyId)}/foglalas`,
    { vendegId },
  );
}

/**
 * Takes a spot away: the caller's, their name-only Vendég's (vendegId), or
 * (the Szervező only) another Felhasználó's.
 */
export async function releaseFoglalas(
  felhasznaloId: string,
  esemenyId: string,
  targetId?: string,
  vendegId = "",
): Promise<void> {
  const query = vendegId
    ? `?vendegId=${encodeURIComponent(vendegId)}`
    : targetId
      ? `?felhasznaloId=${encodeURIComponent(targetId)}`
      : "";
  await callBackend<void>(
    felhasznaloId,
    "DELETE",
    `${base(esemenyId)}/foglalas${query}`,
  );
}

/** The Szervező puts someone into an Ágy, out of their old one. */
export async function moveAlvo(
  felhasznaloId: string,
  esemenyId: string,
  agyId: string,
  targetId: string,
): Promise<void> {
  await callBackend<void>(
    felhasznaloId,
    "PUT",
    `${agyPath(esemenyId, agyId)}/alvo`,
    { felhasznaloId: targetId },
  );
}

/** Marks the caller (or their name-only Vendég) máshol alszik, or unmarks them. */
export async function setMasholAlszik(
  felhasznaloId: string,
  esemenyId: string,
  on: boolean,
  vendegId = "",
): Promise<void> {
  await callBackend<void>(
    felhasznaloId,
    "PUT",
    `${base(esemenyId)}/mashol-alszik`,
    { on, vendegId },
  );
}

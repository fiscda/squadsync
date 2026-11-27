import "server-only";
import { cache } from "react";
import { BackendError, callBackend } from "../backend";
import { requireFelhasznalo } from "../felhasznalo";

// Költségek: Go's /esemeny/{id}/koltsegek routes. The types are kept in
// sync by hand with squadsync-be/internal/httpapi/koltsegek.go.

export type KiadasAllapot = "tervezett" | "kifizetve";

export type Kiadas = {
  id: string;
  description: string;
  /** Whole HUF: the estimate while tervezett, the real amount once kifizetve. */
  amount: number;
  payerId: string;
  payerName: string;
  allapot: KiadasAllapot;
  /** Split equally (true) or in custom amounts (false). */
  egyenlo: boolean;
  createdBy: string;
  /** RFC 3339, UTC */
  createdAt: string;
  /** RFC 3339, UTC */
  updatedAt: string;
};

/** One person's share of a Kiadás. */
export type KiadasResz = {
  felhasznaloId: string;
  /** A name-only Vendég's id (#13), "" otherwise. */
  vendegId: string;
  name: string;
  amount: number;
};

export type KiadasAllas = Kiadas & {
  /** In the stable id order of the holders. */
  reszek: KiadasResz[];
  /** Whether the caller may edit or delete it now. */
  canEdit: boolean;
};

/** A balance over the kifizetve Kiadások; netto = fizetett − resz. */
export type Egyenleg = {
  felhasznaloId: string;
  name: string;
  fizetett: number;
  resz: number;
  netto: number;
};

export type Koltsegek = {
  /** Oldest first. */
  kiadasok: KiadasAllas[];
  /** Ordered by name. */
  egyenlegek: Egyenleg[];
  /** How many Kiadások are still only planned. */
  tervezett: number;
  canAdd: boolean;
};

export type KiadasInput = {
  description: string;
  amount: number;
  /** "" means the caller (adding) or the current payer (editing). */
  payerId: string;
  allapot: KiadasAllapot;
  /** null keeps the current split (editing) or splits among everyone equally (adding). */
  split: {
    egyenlo: boolean;
    /** amount is read only when egyenlo is false. */
    reszek: { felhasznaloId: string; vendegId: string; amount: number }[];
  } | null;
};

function base(esemenyId: string): string {
  return `/esemeny/${encodeURIComponent(esemenyId)}/koltsegek`;
}

/**
 * The Esemény's Költségek, or null if the caller can't see it or it is
 * switched off.
 */
export const getKoltsegek = cache(
  async (esemenyId: string): Promise<Koltsegek | null> => {
    const felhasznalo = await requireFelhasznalo();
    try {
      return await callBackend<Koltsegek>(
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

export async function addKiadas(
  felhasznaloId: string,
  esemenyId: string,
  input: KiadasInput,
): Promise<void> {
  await callBackend(felhasznaloId, "POST", `${base(esemenyId)}/kiadas`, input);
}

export async function updateKiadas(
  felhasznaloId: string,
  esemenyId: string,
  kiadasId: string,
  input: KiadasInput,
): Promise<void> {
  await callBackend<void>(
    felhasznaloId,
    "PUT",
    `${base(esemenyId)}/kiadas/${encodeURIComponent(kiadasId)}`,
    input,
  );
}

export async function deleteKiadas(
  felhasznaloId: string,
  esemenyId: string,
  kiadasId: string,
): Promise<void> {
  await callBackend<void>(
    felhasznaloId,
    "DELETE",
    `${base(esemenyId)}/kiadas/${encodeURIComponent(kiadasId)}`,
  );
}

import type { EsemenyDetail, Resztvevo } from "@/lib/backend";

/** What every Kiegészítő panel gets from the Esemény page. */
export type KiegeszitoPanelProps = {
  esemeny: EsemenyDetail;
  /** The viewer. */
  felhasznaloId: string;
  isSzervezo: boolean;
};

/**
 * The name-only Vendégek the viewer added who are Résztvevők: the viewer
 * claims for them in the Kiegészítők (Go checks it again).
 */
export function sajatVendegResztvevok(
  esemeny: EsemenyDetail,
  felhasznaloId: string,
): Resztvevo[] {
  if (esemeny.allapot !== "veglegesitve") {
    return [];
  }
  return esemeny.resztvevok.filter(
    (r) => r.vendegId !== "" && r.addedBy === felhasznaloId,
  );
}

import type { EsemenyDetail } from "@/lib/backend";

/** What every Kiegészítő panel gets from the Esemény page. */
export type KiegeszitoPanelProps = {
  esemeny: EsemenyDetail;
  /** The viewer. */
  felhasznaloId: string;
  isSzervezo: boolean;
};

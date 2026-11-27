import "server-only";
import { cache } from "react";
import {
  getEsemeny as backendGetEsemeny,
  type EsemenyDetail,
  type EsemenySummary,
  listEsemenyek,
} from "./backend";
import { requireFelhasznalo } from "./felhasznalo";

// Reads are side-effect free and cached for one request.

/** A Csoport's Események, newest first, or null for a non-Tag. */
export const getEsemenyek = cache(
  async (csoportId: string): Promise<EsemenySummary[] | null> => {
    const felhasznalo = await requireFelhasznalo();
    return listEsemenyek(felhasznalo.id, csoportId);
  },
);

/** An Esemény with its Időpontok, or null if the caller can't see it. */
export const getEsemeny = cache(
  async (esemenyId: string): Promise<EsemenyDetail | null> => {
    const felhasznalo = await requireFelhasznalo();
    return backendGetEsemeny(felhasznalo.id, esemenyId);
  },
);

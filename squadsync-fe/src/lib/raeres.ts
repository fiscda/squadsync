import "server-only";
import { cache } from "react";
import {
  getEsemenyRaeres as backendGetEsemenyRaeres,
  getCsoportHeatmap,
  getRaeres,
  type HeatmapDay,
  type IdopontRaeres,
  type RaeresDay,
} from "./backend";
import { requireFelhasznalo } from "./felhasznalo";

// Reads are side-effect free and cached for one request.

/** The caller's own marked days from..to (Budapest dates). */
export const getMyRaeres = cache(
  async (from: string, to: string): Promise<RaeresDay[]> => {
    const felhasznalo = await requireFelhasznalo();
    return getRaeres(felhasznalo.id, from, to);
  },
);

/** How many Tags of a Csoport are free each day, or null for a non-Tag. */
export const getHeatmap = cache(
  async (
    csoportId: string,
    from: string,
    to: string,
  ): Promise<HeatmapDay[] | null> => {
    const felhasznalo = await requireFelhasznalo();
    return getCsoportHeatmap(felhasznalo.id, csoportId, from, to);
  },
);

/** The Ráérés of each Időpont of an Esemény, or null for a non-Tag. */
export const getEsemenyRaeres = cache(
  async (esemenyId: string): Promise<IdopontRaeres[] | null> => {
    const felhasznalo = await requireFelhasznalo();
    return backendGetEsemenyRaeres(felhasznalo.id, esemenyId);
  },
);

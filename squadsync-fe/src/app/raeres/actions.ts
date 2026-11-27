"use server";

import { unstable_rethrow } from "next/navigation";
import { BackendError, type RaeresAllapot, setRaeres } from "@/lib/backend";
import { requireFelhasznalo } from "@/lib/felhasznalo";

export type SetRaeresState = { error?: string };

const dayPattern = /^\d{4}-\d{2}-\d{2}$/;

/**
 * Marks the days from..to (Budapest dates, both included) as szabad or
 * foglalt, or clears them with "". The calendar paints a day or a range in
 * one call. Server actions are public endpoints, so the arguments are
 * checked; Go has the final say on the window.
 */
export async function setRaeresAction(
  from: string,
  to: string,
  allapot: RaeresAllapot | "",
): Promise<SetRaeresState> {
  if (
    !dayPattern.test(from) ||
    !dayPattern.test(to) ||
    !(allapot === "szabad" || allapot === "foglalt" || allapot === "")
  ) {
    throw new Error("invalid raeres arguments");
  }
  const felhasznalo = await requireFelhasznalo();
  try {
    await setRaeres(felhasznalo.id, from, to, allapot);
  } catch (err) {
    unstable_rethrow(err);
    if (err instanceof BackendError && err.status === 400) {
      return {
        error:
          "Ez a nap kívül esik a megjelölhető időszakon (a mai naptól egy évig). Frissítsd az oldalt.",
      };
    }
    throw err;
  }
  return {};
}

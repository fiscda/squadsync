"use server";

import { setKiegeszito } from "@/lib/backend";
import { isKiegeszitoKind } from "./registry";
import { runKiegeszitoChange } from "./run-change";

/** The Szervező switches a Kiegészítő on or off. */
export async function setKiegeszitoAction(
  esemenyId: string,
  kind: string,
  enabled: boolean,
) {
  // Server actions are public endpoints: the bound values can't be trusted.
  if (!isKiegeszitoKind(kind) || typeof enabled !== "boolean") {
    throw new Error("invalid kiegeszito switch");
  }
  await runKiegeszitoChange(esemenyId, (felhasznaloId) =>
    setKiegeszito(felhasznaloId, esemenyId, kind, enabled),
  );
}

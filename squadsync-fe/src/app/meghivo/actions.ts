"use server";

import { redirect } from "next/navigation";
import { joinWithToken } from "@/lib/csoport";
import { discordLoginUrl } from "@/lib/felhasznalo";

/**
 * Sends a visitor through the Discord login and brings them back to the
 * join route of the same link, which makes them a Tag.
 */
export async function signInForMeghivo(token: string) {
  redirect(
    await discordLoginUrl(`/meghivo/${encodeURIComponent(token)}/csatlakozas`),
  );
}

/** Joins the Csoport behind the link for an already logged-in visitor. */
export async function joinAction(token: string) {
  const result = await joinWithToken(token);
  if (result.status === "joined" || result.status === "already-tag") {
    redirect(`/csoportok/${result.csoportId}`);
  }
  // Refused: the link's page says why.
  redirect(`/meghivo/${encodeURIComponent(token)}`);
}

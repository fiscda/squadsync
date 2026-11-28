"use server";

import { redirect } from "next/navigation";
import { discordLoginUrl } from "@/lib/felhasznalo";
import { joinEsemenyWithToken } from "@/lib/vendeg";

/**
 * Sends a visitor through the Discord login and brings them back to the
 * join route of the same link, which makes them a Vendég.
 */
export async function signInForEsemenyMeghivo(token: string) {
  redirect(
    await discordLoginUrl(
      `/esemeny-meghivo/${encodeURIComponent(token)}/csatlakozas`,
    ),
  );
}

/** Joins the Esemény behind the link for an already logged-in visitor. */
export async function joinEsemenyAction(token: string) {
  const result = await joinEsemenyWithToken(token);
  if (result.status === "ok") {
    redirect(`/esemenyek/${result.esemenyId}`);
  }
  // Refused: the link's page says why.
  redirect(`/esemeny-meghivo/${encodeURIComponent(token)}`);
}

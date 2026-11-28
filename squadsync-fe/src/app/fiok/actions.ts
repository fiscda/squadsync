"use server";

import { revalidatePath } from "next/cache";
import { unstable_rethrow } from "next/navigation";
import {
  BackendError,
  disconnectGoogleNaptar,
  unlinkDiscordAccount,
} from "@/lib/backend";
import { requireFelhasznalo } from "@/lib/felhasznalo";

/**
 * Unlinks one of the Felhasználó's extra Discord accounts. It then gets no
 * more DMs, and the bot treats it as unknown. The main account has no
 * button; a 404 or 409 means the page is stale, so just show it again.
 */
export async function unlinkDiscordAccountAction(discordId: string) {
  const felhasznalo = await requireFelhasznalo();
  try {
    await unlinkDiscordAccount(felhasznalo.id, discordId);
  } catch (err) {
    unstable_rethrow(err);
    if (
      !(
        err instanceof BackendError &&
        (err.status === 404 || err.status === 409)
      )
    ) {
      throw err;
    }
  }
  revalidatePath("/fiok");
}

/**
 * Disconnects Google Calendar: Go stops syncing, forgets the token and
 * revokes it. The "Squad Sync" calendar stays in their Google account.
 */
export async function disconnectGoogleNaptarAction() {
  const felhasznalo = await requireFelhasznalo();
  await disconnectGoogleNaptar(felhasznalo.id);
  revalidatePath("/fiok");
}

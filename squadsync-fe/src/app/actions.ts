"use server";

import { headers } from "next/headers";
import { redirect } from "next/navigation";
import { auth } from "@/lib/auth";
import { ensureFelhasznalo } from "@/lib/felhasznalo";

export async function signInWithDiscord() {
  const { url } = await auth.api.signInSocial({
    // After Discord, Better Auth redirects to this route handler, which
    // makes sure Go knows the Felhasználó.
    body: { provider: "discord", callbackURL: "/login/complete" },
    headers: await headers(),
  });
  if (!url) {
    throw new Error("Better Auth returned no Discord authorization URL");
  }
  redirect(url);
}

/** Retries telling Go about the logged-in Felhasználó. */
export async function retryFirstLogin() {
  await ensureFelhasznalo();
  redirect("/");
}

export async function signOut() {
  await auth.api.signOut({ headers: await headers() });
  redirect("/");
}

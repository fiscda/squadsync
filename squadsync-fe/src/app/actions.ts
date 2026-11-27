"use server";

import { headers } from "next/headers";
import { redirect } from "next/navigation";
import { auth } from "@/lib/auth";
import { discordLoginUrl, ensureFelhasznalo } from "@/lib/felhasznalo";

export async function signInWithDiscord() {
  // After Discord, Better Auth redirects to this route handler, which makes
  // sure Go knows the Felhasználó.
  redirect(await discordLoginUrl("/login/complete"));
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

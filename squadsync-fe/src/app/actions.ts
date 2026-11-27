"use server";

import { headers } from "next/headers";
import { redirect } from "next/navigation";
import { auth } from "@/lib/auth";

export async function signInWithDiscord() {
  const { url } = await auth.api.signInSocial({
    body: { provider: "discord", callbackURL: "/" },
    headers: await headers(),
  });
  if (!url) {
    throw new Error("Better Auth returned no Discord authorization URL");
  }
  redirect(url);
}

export async function signOut() {
  await auth.api.signOut({ headers: await headers() });
  redirect("/");
}

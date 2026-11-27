// Better Auth: Discord login for the web app. Also loaded by the Better Auth
// CLI (`bun run auth:migrate`), so it uses relative imports only.
import { betterAuth } from "better-auth";
import { nextCookies } from "better-auth/next-js";
import { firstLogin } from "./backend";
import { db } from "./db";

export const auth = betterAuth({
  // BETTER_AUTH_SECRET and BETTER_AUTH_URL are read from the environment.
  database: db,
  socialProviders: {
    discord: {
      clientId: process.env.DISCORD_CLIENT_ID ?? "",
      clientSecret: process.env.DISCORD_CLIENT_SECRET ?? "",
    },
  },
  databaseHooks: {
    account: {
      create: {
        // Runs once, after the first Discord login of a new user has been
        // committed. Go then creates the Felhasználó (ADR-0002).
        after: async (account) => {
          if (account.providerId !== "discord") {
            return;
          }
          try {
            await firstLogin(account.userId, {
              discordId: account.accountId,
              name: userName(account.userId),
            });
          } catch (err) {
            // Don't fail the login over it: the page retries the call when
            // Go doesn't know the Felhasználó yet (see felhasznalo.ts).
            console.error("first login: Go call failed", err);
          }
        },
      },
    },
  },
  plugins: [nextCookies()], // must stay last
});

// Better Auth's `user` table belongs to Next.js, so reading it here is fine
// (only Go must never touch it). The hook runs after the user is committed.
function userName(userId: string): string {
  const row = db.prepare('SELECT name FROM "user" WHERE id = ?').get(userId);
  return typeof row?.name === "string" ? row.name : "";
}

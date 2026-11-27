// Better Auth: Discord login for the web app. Also loaded by the Better Auth
// CLI (`bun run auth:migrate`), so it uses relative imports only.
import { betterAuth } from "better-auth";
import { nextCookies } from "better-auth/next-js";
import { checkWebLogin, firstLogin } from "./backend";
import { db } from "./db";
import { refuseLinkedDiscordAccount } from "./web-login-guard";

export const auth = betterAuth({
  // BETTER_AUTH_SECRET and BETTER_AUTH_URL are read from the environment.
  database: db,
  user: {
    // Only the main Discord account logs in to the web. Go knows which
    // accounts are linked ones; login with one of them is refused, naming
    // the main account (ADR-0002). This runs before Better Auth creates a
    // new user, and before it creates the session of an existing user (an
    // orphan user from before the account was linked), so neither happens.
    // The refusal redirects to the errorCallbackURL (see discordLoginUrl).
    validateUserInfo: ({ source }) =>
      refuseLinkedDiscordAccount(source, checkWebLogin),
  },
  account: {
    // Better Auth handles only web login with the main Discord account. Go
    // owns linking further Discord accounts (ADR-0002), so Better Auth must
    // never attach a second account to a user.
    accountLinking: { enabled: false },
  },
  socialProviders: {
    discord: {
      clientId: process.env.DISCORD_CLIENT_ID ?? "",
      clientSecret: process.env.DISCORD_CLIENT_SECRET ?? "",
      // Discord returns no email for phone-only accounts, and Better Auth
      // refuses a login without one. We never email anyone, so a unique
      // placeholder on the reserved .invalid domain is enough.
      mapProfileToUser: (profile) => ({
        email: profile.email ?? `${profile.id}@discord.placeholder.invalid`,
        emailVerified: profile.email ? profile.verified : false,
      }),
    },
  },
  databaseHooks: {
    account: {
      create: {
        // Runs once, after the first Discord login of a new user has been
        // committed. Go then creates the Felhasználó (ADR-0002).
        after: async (account, ctx) => {
          if (account.providerId !== "discord") {
            return;
          }
          try {
            const user = await ctx?.context.internalAdapter.findUserById(
              account.userId,
            );
            await firstLogin(account.userId, {
              discordId: account.accountId,
              name: user?.name ?? "",
            });
          } catch (err) {
            // Don't fail the login over it: the login callback repeats the
            // call (ensureFelhasznalo in felhasznalo.ts).
            console.error("first login: Go call failed", err);
          }
        },
      },
    },
  },
  plugins: [nextCookies()], // must stay last
});

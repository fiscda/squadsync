// Run with `bun test` (also part of `make test`).
import assert from "node:assert/strict";
import { test } from "node:test";
import type { WebLoginCheck } from "./backend";
import {
  LINKED_ACCOUNT_ERROR,
  LOGIN_CHECK_FAILED_ERROR,
  type LoginSource,
  refuseLinkedDiscordAccount,
  signDiscordId,
  verifySignedDiscordId,
} from "./web-login-guard";

const SECRET = "test-secret";
const ANNA = "100000000000000001";
const ANNA_PHONE = "100000000000000002";
const STRANGER = "100000000000000003";

// Go's answers, as the web-login route gives them.
const go = async (discordId: string): Promise<WebLoginCheck> =>
  discordId === ANNA_PHONE
    ? {
        allowed: false,
        message: "Ez a fiók Anna-hez van kapcsolva, lépj be azzal.",
      }
    : { allowed: true, message: "" };

// What Better Auth 1.7.28 passes to validateUserInfo for a Discord login:
// the raw Discord profile, whose `id` is the Discord user id, is in
// source.oauth.profile (node_modules/better-auth/dist/api/routes/callback.mjs:155).
function discordLogin(action: string, discordId: string): LoginSource {
  return {
    action,
    method: "oauth",
    oauth: { providerId: "discord", profile: { id: discordId } },
  };
}

test("a linked account is refused before its user is created", async () => {
  const refusal = await refuseLinkedDiscordAccount(
    discordLogin("create-user", ANNA_PHONE),
    go,
    SECRET,
  );
  assert.equal(refusal?.error, LINKED_ACCOUNT_ERROR);
  // No text travels in the URL: only the signed Discord id.
  assert.equal(
    verifySignedDiscordId(refusal?.errorDescription ?? "", SECRET),
    ANNA_PHONE,
  );
});

test("a linked account with an orphan user is refused before its session", async () => {
  const refusal = await refuseLinkedDiscordAccount(
    discordLogin("sign-in", ANNA_PHONE),
    go,
    SECRET,
  );
  assert.equal(refusal?.error, LINKED_ACCOUNT_ERROR);
});

test("main and unknown accounts log in", async () => {
  for (const action of ["create-user", "sign-in"]) {
    for (const id of [ANNA, STRANGER]) {
      assert.equal(
        await refuseLinkedDiscordAccount(discordLogin(action, id), go, SECRET),
        undefined,
      );
    }
  }
});

test("the login is refused when Go can't be asked", async () => {
  const logError = console.error;
  console.error = () => {}; // the guard logs the failure; keep output clean
  try {
    const refusal = await refuseLinkedDiscordAccount(
      discordLogin("create-user", ANNA),
      async () => {
        throw new Error("connection refused");
      },
      SECRET,
    );
    assert.deepEqual(refusal, { error: LOGIN_CHECK_FAILED_ERROR });
  } finally {
    console.error = logError;
  }
});

test("other providers and actions are not asked about", async () => {
  const never = async (): Promise<WebLoginCheck> => {
    throw new Error("Go must not be asked");
  };
  assert.equal(
    await refuseLinkedDiscordAccount(
      { action: "create-user", method: "email-password" },
      never,
      SECRET,
    ),
    undefined,
  );
  assert.equal(
    await refuseLinkedDiscordAccount(
      discordLogin("link-account", ANNA_PHONE),
      never,
      SECRET,
    ),
    undefined,
  );
});

test("forged or malformed signed ids are rejected", () => {
  const token = signDiscordId(ANNA_PHONE, SECRET);
  assert.equal(verifySignedDiscordId(token, SECRET), ANNA_PHONE);
  assert.equal(verifySignedDiscordId(token, "other-secret"), null);
  assert.equal(
    verifySignedDiscordId(`${STRANGER}.${token.split(".")[1]}`, SECRET),
    null,
  );
  assert.equal(verifySignedDiscordId(ANNA_PHONE, SECRET), null);
  assert.equal(
    verifySignedDiscordId(signDiscordId("Ez a szöveg", SECRET), SECRET),
    null,
  );
  assert.equal(verifySignedDiscordId(token, ""), null);
});

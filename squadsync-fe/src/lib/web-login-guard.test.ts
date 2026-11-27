// Run with `bun test` (also part of `make test`).
import assert from "node:assert/strict";
import { test } from "node:test";
import type { WebLoginCheck } from "./backend";
import {
  LINKED_ACCOUNT_ERROR,
  LOGIN_CHECK_FAILED_ERROR,
  type LoginSource,
  refuseLinkedDiscordAccount,
} from "./web-login-guard";

// Go's answers, as the web-login route gives them.
const go = async (discordId: string): Promise<WebLoginCheck> =>
  discordId === "d-anna-phone"
    ? {
        allowed: false,
        message: "Ez a fiók Anna-hez van kapcsolva, lépj be azzal.",
      }
    : { allowed: true, message: "" };

function discordLogin(action: string, discordId: string): LoginSource {
  return {
    action,
    method: "oauth",
    oauth: { providerId: "discord", profile: { id: discordId } },
  };
}

test("a linked account is refused before its user is created", async () => {
  assert.deepEqual(
    await refuseLinkedDiscordAccount(
      discordLogin("create-user", "d-anna-phone"),
      go,
    ),
    {
      error: LINKED_ACCOUNT_ERROR,
      errorDescription: "Ez a fiók Anna-hez van kapcsolva, lépj be azzal.",
    },
  );
});

test("a linked account with an orphan user is refused before its session", async () => {
  const refusal = await refuseLinkedDiscordAccount(
    discordLogin("sign-in", "d-anna-phone"),
    go,
  );
  assert.equal(refusal?.error, LINKED_ACCOUNT_ERROR);
});

test("main and unknown accounts log in", async () => {
  for (const action of ["create-user", "sign-in"]) {
    for (const id of ["d-anna", "d-stranger"]) {
      assert.equal(
        await refuseLinkedDiscordAccount(discordLogin(action, id), go),
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
      discordLogin("create-user", "d-anna"),
      async () => {
        throw new Error("connection refused");
      },
    );
    assert.equal(refusal?.error, LOGIN_CHECK_FAILED_ERROR);
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
    ),
    undefined,
  );
  assert.equal(
    await refuseLinkedDiscordAccount(
      discordLogin("link-account", "d-anna-phone"),
      never,
    ),
    undefined,
  );
});

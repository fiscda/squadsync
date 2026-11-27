"use client";

import { useActionState, useState } from "react";
import { buttonClass, secondaryButtonClass } from "@/lib/ui";
import {
  type CsoportActionState,
  leaveCsoportAction,
  removeTagAction,
  renameCsoportAction,
  setAdminAction,
} from "../actions";

function ErrorText({ error }: { error?: string }) {
  return error ? (
    <p role="alert" className="text-sm text-red-600 dark:text-red-400">
      {error}
    </p>
  ) : null;
}

const dangerButtonClass =
  "rounded-full bg-red-600 px-4 py-2 text-sm font-medium text-white transition-opacity hover:opacity-80";

/** Admins rename the Csoport. */
export function RenameForm({
  csoportId,
  name,
}: {
  csoportId: string;
  name: string;
}) {
  const [state, action, pending] = useActionState<CsoportActionState, FormData>(
    renameCsoportAction.bind(null, csoportId),
    {},
  );

  return (
    <form action={action} className="flex flex-col gap-3">
      <label htmlFor="csoport-name" className="font-medium">
        Csoport neve
      </label>
      <div className="flex flex-col gap-3 sm:flex-row">
        <input
          id="csoport-name"
          name="name"
          required
          maxLength={60}
          defaultValue={name}
          autoComplete="off"
          className="min-w-0 flex-1 rounded-full border border-zinc-300 bg-transparent px-4 py-3 dark:border-zinc-700"
        />
        <button type="submit" disabled={pending} className={buttonClass}>
          {pending ? "Mentés…" : "Átnevezés"}
        </button>
      </div>
      <ErrorText error={state.error} />
    </form>
  );
}

/**
 * What an Admin can do with one Tag: give or take the Admin role, and remove
 * them (after an inline confirmation). Removal is for other Tags only; the
 * Admin leaves through "Kilépés a csoportból".
 */
export function TagControls({
  csoportId,
  tagId,
  tagName,
  admin,
  self,
}: {
  csoportId: string;
  tagId: string;
  tagName: string;
  admin: boolean;
  self: boolean;
}) {
  const [confirming, setConfirming] = useState(false);
  const [roleState, roleAction, rolePending] = useActionState<
    CsoportActionState,
    FormData
  >(() => setAdminAction(csoportId, tagId, !admin), {});
  const [removeState, removeAction, removePending] = useActionState<
    CsoportActionState,
    FormData
  >(async () => {
    const result = await removeTagAction(csoportId, tagId);
    setConfirming(false);
    return result;
  }, {});

  return (
    <div className="flex flex-col gap-2">
      <div className="flex flex-wrap items-center gap-2">
        <form action={roleAction}>
          <button
            type="submit"
            disabled={rolePending}
            className={secondaryButtonClass}
          >
            {admin ? "Admin rangot elvesz" : "Admin rangot ad"}
          </button>
        </form>
        {!self &&
          (confirming ? (
            <form action={removeAction} className="flex items-center gap-2">
              <span className="text-sm">Biztosan eltávolítod: {tagName}?</span>
              <button
                type="submit"
                disabled={removePending}
                className={dangerButtonClass}
              >
                Igen, eltávolítom
              </button>
              <button
                type="button"
                onClick={() => setConfirming(false)}
                className={secondaryButtonClass}
              >
                Mégsem
              </button>
            </form>
          ) : (
            <button
              type="button"
              onClick={() => setConfirming(true)}
              className={secondaryButtonClass}
            >
              Eltávolítás
            </button>
          ))}
      </div>
      <ErrorText error={roleState.error} />
      <ErrorText error={removeState.error} />
    </div>
  );
}

/** Any Tag leaves the Csoport, after an inline confirmation. */
export function LeaveControl({ csoportId }: { csoportId: string }) {
  const [confirming, setConfirming] = useState(false);
  const [state, action, pending] = useActionState<CsoportActionState, FormData>(
    () => leaveCsoportAction(csoportId),
    {},
  );

  return (
    <div className="flex flex-col gap-2">
      {confirming ? (
        <form action={action} className="flex flex-wrap items-center gap-2">
          <span className="text-sm">Biztosan kilépsz a csoportból?</span>
          <button
            type="submit"
            disabled={pending}
            className={dangerButtonClass}
          >
            Igen, kilépek
          </button>
          <button
            type="button"
            onClick={() => setConfirming(false)}
            className={secondaryButtonClass}
          >
            Mégsem
          </button>
        </form>
      ) : (
        <div>
          <button
            type="button"
            onClick={() => setConfirming(true)}
            className={secondaryButtonClass}
          >
            Kilépés a csoportból
          </button>
        </div>
      )}
      <ErrorText error={state.error} />
    </div>
  );
}

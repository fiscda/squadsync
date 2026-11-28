"use client";

import Link from "next/link";
import { useActionState, useState } from "react";
import type { EsemenyRef, SzervezoEsemeny } from "@/lib/backend";
import { buttonClass, linkClass, secondaryButtonClass } from "@/lib/ui";
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
 *
 * A Tag who is the Szervező of active Események can only be removed once
 * each of them has a new Szervező, picked here from the Tags with a
 * Visszajelzés on it. An Admin can't cancel someone else's Esemény, so one
 * with nobody to take it blocks the removal.
 */
export function TagControls({
  csoportId,
  tagId,
  tagName,
  admin,
  self,
  szervezoEsemenyek,
}: {
  csoportId: string;
  tagId: string;
  tagName: string;
  admin: boolean;
  self: boolean;
  szervezoEsemenyek: SzervezoEsemeny[];
}) {
  const [confirming, setConfirming] = useState(false);
  const [roleState, roleAction, rolePending] = useActionState<
    CsoportActionState,
    FormData
  >(() => setAdminAction(csoportId, tagId, !admin), {});
  const [removeState, removeAction, removePending] = useActionState<
    CsoportActionState,
    FormData
  >(async (_previous, formData) => {
    const szervezok: Record<string, string> = {};
    for (const esemeny of szervezoEsemenyek) {
      szervezok[esemeny.id] = String(formData.get(esemeny.id) ?? "");
    }
    const result = await removeTagAction(csoportId, tagId, szervezok);
    if (result.error === undefined) {
      setConfirming(false);
    }
    return result;
  }, {});
  const stuck = szervezoEsemenyek.filter((e) => e.utodok.length === 0);

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
        {!self && !confirming && (
          <button
            type="button"
            onClick={() => setConfirming(true)}
            className={secondaryButtonClass}
          >
            Eltávolítás
          </button>
        )}
      </div>
      {!self && confirming && (
        <form
          action={removeAction}
          className="flex flex-col gap-3 rounded-2xl border border-red-300 p-3 dark:border-red-900"
        >
          <span className="text-sm">Biztosan eltávolítod: {tagName}?</span>
          {szervezoEsemenyek.length > 0 && (
            <SzervezoPickers
              tagName={tagName}
              szervezoEsemenyek={szervezoEsemenyek}
            />
          )}
          <div className="flex flex-wrap items-center gap-2">
            <button
              type="submit"
              disabled={removePending || stuck.length > 0}
              className={`${dangerButtonClass} disabled:opacity-50`}
            >
              {szervezoEsemenyek.length > 0
                ? "Igen, eltávolítom és átadom a szervezést"
                : "Igen, eltávolítom"}
            </button>
            <button
              type="button"
              onClick={() => setConfirming(false)}
              className={secondaryButtonClass}
            >
              Mégsem
            </button>
          </div>
        </form>
      )}
      <ErrorText error={roleState.error} />
      <ErrorText error={removeState.error} />
    </div>
  );
}

/** One select per Esemény: who becomes its Szervező instead of the Tag. */
function SzervezoPickers({
  tagName,
  szervezoEsemenyek,
}: {
  tagName: string;
  szervezoEsemenyek: SzervezoEsemeny[];
}) {
  return (
    <fieldset className="flex flex-col gap-3">
      <legend className="text-sm text-zinc-600 dark:text-zinc-400">
        {tagName} az alábbi aktív események szervezője. Válassz mindegyikhez új
        szervezőt azok közül, akik válaszoltak rá.
      </legend>
      {szervezoEsemenyek.map((esemeny) => (
        <div key={esemeny.id} className="flex flex-col gap-1">
          <label htmlFor={`szervezo-${esemeny.id}`} className="text-sm">
            <Link href={`/esemenyek/${esemeny.id}`} className={linkClass}>
              {esemeny.title}
            </Link>{" "}
            <span className="text-zinc-500">
              ({allapotText[esemeny.allapot]})
            </span>
          </label>
          {esemeny.utodok.length === 0 ? (
            <p
              id={`szervezo-${esemeny.id}`}
              role="alert"
              className="text-sm text-red-600 dark:text-red-400"
            >
              Erre az eseményre senki más nem válaszolt, így nincs, akinek át
              lehetne adni. Csak a szervezője mondhatja le, ezért addig{" "}
              {tagName} nem távolítható el.
            </p>
          ) : (
            <select
              id={`szervezo-${esemeny.id}`}
              name={esemeny.id}
              required
              defaultValue=""
              className="rounded-full border border-zinc-300 bg-transparent px-4 py-2 text-sm dark:border-zinc-700"
            >
              <option value="" disabled>
                Új szervező…
              </option>
              {esemeny.utodok.map((tag) => (
                <option key={tag.felhasznaloId} value={tag.felhasznaloId}>
                  {tag.name}
                </option>
              ))}
            </select>
          )}
        </div>
      ))}
    </fieldset>
  );
}

const allapotText: Record<EsemenyRef["allapot"], string> = {
  egyeztetes: "egyeztetés",
  veglegesitve: "véglegesítve",
  elmaradt: "elmaradt",
  lezajlott: "lezajlott",
};

/**
 * Any Tag leaves the Csoport, after an inline confirmation. While they are
 * the Szervező of active Események they can't: those are listed, each with a
 * link, so they can hand them over or cancel them first.
 */
export function LeaveControl({
  csoportId,
  blocking,
}: {
  csoportId: string;
  blocking: EsemenyRef[];
}) {
  const [confirming, setConfirming] = useState(false);
  const [state, action, pending] = useActionState<CsoportActionState, FormData>(
    () => leaveCsoportAction(csoportId),
    {},
  );

  if (blocking.length > 0) {
    return (
      <div className="flex flex-col gap-2 rounded-2xl border border-amber-300 bg-amber-50 p-4 text-sm text-amber-900 dark:border-amber-800 dark:bg-amber-950 dark:text-amber-200">
        <p className="font-medium">Még nem léphetsz ki a csoportból.</p>
        <p>
          Te vagy a szervezője az alábbi eseményeknek. Add át mindegyik
          szervezését egy másik tagnak, aki válaszolt rá, vagy mondd le az
          eseményt (ha senki más nem válaszolt rá, csak a lemondás marad).
        </p>
        <ul className="list-inside list-disc">
          {blocking.map((esemeny) => (
            <li key={esemeny.id}>
              <Link href={`/esemenyek/${esemeny.id}`} className={linkClass}>
                {esemeny.title}
              </Link>{" "}
              ({allapotText[esemeny.allapot]})
            </li>
          ))}
        </ul>
        <ErrorText error={state.error} />
      </div>
    );
  }

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

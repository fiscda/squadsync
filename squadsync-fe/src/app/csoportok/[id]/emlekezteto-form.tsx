"use client";

import { useActionState } from "react";
import { EMLEKEZTETO_VALASZTEK } from "@/lib/emlekezteto";
import { buttonClass } from "@/lib/ui";
import { type CsoportActionState, setEmlekeztetokAction } from "../actions";

/** Admins pick when the Csoport's Emlékeztetők go out. */
export function EmlekeztetoForm({
  csoportId,
  percek,
}: {
  csoportId: string;
  percek: number[];
}) {
  const [state, action, pending] = useActionState<CsoportActionState, FormData>(
    setEmlekeztetokAction.bind(null, csoportId),
    {},
  );
  return (
    <form action={action} className="flex flex-col gap-3">
      <fieldset className="flex flex-col gap-2">
        <legend className="sr-only">Emlékeztetők időzítése</legend>
        {EMLEKEZTETO_VALASZTEK.map((v) => (
          <label key={v.perc} className="flex items-center gap-2 text-sm">
            <input
              type="checkbox"
              name="perc"
              value={v.perc}
              defaultChecked={percek.includes(v.perc)}
            />
            {v.label}
          </label>
        ))}
      </fieldset>
      <button
        type="submit"
        disabled={pending}
        className={`${buttonClass} self-start`}
      >
        {pending ? "Mentés…" : "Mentés"}
      </button>
      {state.error ? (
        <p role="alert" className="text-sm text-red-600 dark:text-red-400">
          {state.error}
        </p>
      ) : null}
    </form>
  );
}

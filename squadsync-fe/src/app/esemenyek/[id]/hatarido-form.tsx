"use client";

import { useActionState } from "react";
import { buttonClass, secondaryButtonClass } from "@/lib/ui";
import { type HataridoState, setHataridoAction } from "../actions";

const inputClass =
  "min-w-0 rounded-xl border border-zinc-300 bg-transparent px-3 py-2 dark:border-zinc-700";

/**
 * The Szervező sets, changes or clears the voting Határidő while the
 * Esemény is in Egyeztetés. The current one comes in as Budapest date and
 * time input values.
 */
export function HataridoForm({
  esemenyId,
  date,
  time,
}: {
  esemenyId: string;
  /** "" when there is no Határidő. */
  date: string;
  time: string;
}) {
  const [state, action, pending] = useActionState<HataridoState, FormData>(
    setHataridoAction.bind(null, esemenyId),
    {},
  );
  return (
    <form action={action} className="flex flex-col gap-2">
      <span className="text-sm font-medium">Szavazási határidő</span>
      <p className="text-sm text-zinc-600 dark:text-zinc-400">
        Amikor lejár, Discordon üzenetet kapsz az időpontok állásáról, és onnan
        is véglegesíthetsz. A szavazás ezután is nyitva marad.
      </p>
      <div className="flex flex-wrap items-center gap-2">
        <input
          type="date"
          name="hataridoDate"
          aria-label="Határidő napja"
          defaultValue={date}
          className={inputClass}
        />
        <input
          type="time"
          name="hataridoTime"
          aria-label="Határidő ideje"
          defaultValue={time}
          className={inputClass}
        />
        <button
          type="submit"
          name="intent"
          value="set"
          disabled={pending}
          className={buttonClass}
        >
          {date === "" ? "Beállítás" : "Módosítás"}
        </button>
        {date !== "" ? (
          <button
            type="submit"
            name="intent"
            value="clear"
            disabled={pending}
            className={secondaryButtonClass}
          >
            Törlés
          </button>
        ) : null}
      </div>
      {state.error ? (
        <p role="alert" className="text-sm text-red-600 dark:text-red-400">
          {state.error}
        </p>
      ) : null}
    </form>
  );
}

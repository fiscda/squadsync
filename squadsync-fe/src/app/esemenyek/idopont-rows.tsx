"use client";

import { useState } from "react";
import { secondaryButtonClass } from "@/lib/ui";
import { MAX_IDOPONTOK } from "./limits";

type IdopontRow = {
  key: number;
  egeszNapos: boolean;
  startDate: string;
  startTime: string;
  endDate: string;
  endTime: string;
};

export const inputClass =
  "min-w-0 rounded-xl border border-zinc-300 bg-transparent px-3 py-2 dark:border-zinc-700";

/**
 * The Időpont fields shared by the propose form and the "+ Időpont
 * hozzáadása" form: a list of rows (or, with `single`, exactly one) whose
 * fields idopontokFromForm reads on the server. Inputs are controlled so
 * that a refused submit keeps them; remount it (change its key) to clear
 * it after a successful one.
 */
export function IdopontRows({
  single = false,
  maxRows = MAX_IDOPONTOK,
}: {
  /** One Időpont, with no list controls. */
  single?: boolean;
  /** At most this many rows (the Esemény's cap, when adding). */
  maxRows?: number;
}) {
  const [rows, setRows] = useState<IdopontRow[]>([emptyRow(0)]);

  const setRow = (key: number, change: Partial<IdopontRow>) =>
    setRows((rs) => rs.map((r) => (r.key === key ? { ...r, ...change } : r)));

  return (
    <fieldset className="flex flex-col gap-3">
      <legend className="mb-1 font-medium">
        {single ? "Új időpont" : "Időpontok"}
      </legend>
      <p className="text-sm text-zinc-600 dark:text-zinc-400">
        Budapesti idő szerint. A befejezés nem kötelező, és több nappal későbbi
        is lehet. Egész napos időpontnál csak a napokat add meg.
      </p>
      {rows.map((row, i) => (
        <div
          key={row.key}
          className="flex flex-col gap-3 rounded-2xl border border-zinc-200 p-4 dark:border-zinc-800"
        >
          <div className="flex items-center justify-between">
            <span className="text-sm font-medium">
              {single ? "Időpont" : `${i + 1}. időpont`}
            </span>
            {rows.length > 1 ? (
              <button
                type="button"
                onClick={() =>
                  setRows((rs) => rs.filter((r) => r.key !== row.key))
                }
                className="text-sm text-zinc-500 underline underline-offset-4 hover:opacity-80"
              >
                Törlés
              </button>
            ) : null}
          </div>
          {/* Every row sends every field, so the server can line them up. */}
          <input
            type="hidden"
            name="egeszNapos"
            value={row.egeszNapos ? "1" : "0"}
          />
          <label className="flex items-center gap-2 text-sm">
            <input
              type="checkbox"
              checked={row.egeszNapos}
              onChange={(e) =>
                setRow(row.key, { egeszNapos: e.target.checked })
              }
            />
            Egész napos
          </label>
          <div
            className={`grid items-center gap-2 ${
              row.egeszNapos
                ? "grid-cols-[auto_1fr]"
                : "grid-cols-[auto_1fr_1fr]"
            }`}
          >
            <span className="text-sm text-zinc-500">
              {row.egeszNapos ? "Első nap" : "Kezdés"}
            </span>
            <input
              type="date"
              name="startDate"
              required
              aria-label={`${i + 1}. időpont kezdő napja`}
              value={row.startDate}
              onChange={(e) => setRow(row.key, { startDate: e.target.value })}
              className={inputClass}
            />
            <input
              type="time"
              name="startTime"
              required={!row.egeszNapos}
              hidden={row.egeszNapos}
              aria-label={`${i + 1}. időpont kezdő ideje`}
              value={row.startTime}
              onChange={(e) => setRow(row.key, { startTime: e.target.value })}
              className={inputClass}
            />
            <span className="text-sm text-zinc-500">
              {row.egeszNapos ? "Utolsó nap" : "Vége"}
            </span>
            <input
              type="date"
              name="endDate"
              aria-label={`${i + 1}. időpont záró napja`}
              min={row.startDate || undefined}
              value={row.endDate}
              onChange={(e) => setRow(row.key, { endDate: e.target.value })}
              className={inputClass}
            />
            <input
              type="time"
              name="endTime"
              hidden={row.egeszNapos}
              aria-label={`${i + 1}. időpont záró ideje`}
              value={row.endTime}
              onChange={(e) => setRow(row.key, { endTime: e.target.value })}
              className={inputClass}
            />
          </div>
        </div>
      ))}
      {!single && rows.length < maxRows ? (
        <button
          type="button"
          onClick={() =>
            setRows((rs) => [
              ...rs,
              emptyRow(Math.max(...rs.map((r) => r.key)) + 1),
            ])
          }
          className={`${secondaryButtonClass} self-start`}
        >
          + Újabb időpont
        </button>
      ) : null}
    </fieldset>
  );
}

function emptyRow(key: number): IdopontRow {
  return {
    key,
    egeszNapos: false,
    startDate: "",
    startTime: "",
    endDate: "",
    endTime: "",
  };
}

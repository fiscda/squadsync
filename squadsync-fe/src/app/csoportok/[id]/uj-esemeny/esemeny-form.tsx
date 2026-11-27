"use client";

import { useActionState, useState } from "react";
import {
  type ProposeEsemenyState,
  proposeEsemenyAction,
} from "@/app/esemenyek/actions";
import { buttonClass, secondaryButtonClass } from "@/lib/ui";

type IdopontRow = {
  key: number;
  egeszNapos: boolean;
  startDate: string;
  startTime: string;
  endDate: string;
  endTime: string;
};

const MAX_IDOPONTOK = 20;

const inputClass =
  "min-w-0 rounded-xl border border-zinc-300 bg-transparent px-3 py-2 dark:border-zinc-700";

/**
 * The propose form. A client component only for the growing list of
 * Időpontok. Inputs are controlled so that a refused submit keeps them.
 */
export function EsemenyForm({ csoportId }: { csoportId: string }) {
  const [state, action, pending] = useActionState<
    ProposeEsemenyState,
    FormData
  >(proposeEsemenyAction.bind(null, csoportId), {});
  const [fields, setFields] = useState({
    title: "",
    description: "",
    helyszinName: "",
    helyszinUrl: "",
  });
  const [rows, setRows] = useState<IdopontRow[]>([emptyRow(0)]);

  const setField = (name: keyof typeof fields, value: string) =>
    setFields((f) => ({ ...f, [name]: value }));
  const setRow = (key: number, change: Partial<IdopontRow>) =>
    setRows((rs) => rs.map((r) => (r.key === key ? { ...r, ...change } : r)));

  return (
    <form action={action} className="flex flex-col gap-6">
      <label className="flex flex-col gap-1">
        <span className="font-medium">Cím</span>
        <input
          name="title"
          required
          maxLength={100}
          placeholder="Pl. Társasjáték-est"
          autoComplete="off"
          value={fields.title}
          onChange={(e) => setField("title", e.target.value)}
          className={inputClass}
        />
      </label>

      <label className="flex flex-col gap-1">
        <span className="font-medium">
          Leírás <span className="text-sm text-zinc-500">(nem kötelező)</span>
        </span>
        <textarea
          name="description"
          maxLength={2000}
          rows={3}
          value={fields.description}
          onChange={(e) => setField("description", e.target.value)}
          className={inputClass}
        />
      </label>

      <fieldset className="flex flex-col gap-3">
        <legend className="mb-1 font-medium">
          Helyszín <span className="text-sm text-zinc-500">(nem kötelező)</span>
        </legend>
        <input
          name="helyszinName"
          maxLength={200}
          placeholder="Pl. Anna lakása"
          aria-label="Helyszín neve"
          autoComplete="off"
          value={fields.helyszinName}
          onChange={(e) => setField("helyszinName", e.target.value)}
          className={inputClass}
        />
        <input
          name="helyszinUrl"
          type="url"
          maxLength={2000}
          placeholder="Google Maps link (https://…)"
          aria-label="Google Maps link"
          autoComplete="off"
          value={fields.helyszinUrl}
          onChange={(e) => setField("helyszinUrl", e.target.value)}
          className={inputClass}
        />
      </fieldset>

      <fieldset className="flex flex-col gap-3">
        <legend className="mb-1 font-medium">Időpontok</legend>
        <p className="text-sm text-zinc-600 dark:text-zinc-400">
          Budapesti idő szerint. A befejezés nem kötelező, és több nappal
          későbbi is lehet. Egész napos időpontnál csak a napokat add meg.
        </p>
        {rows.map((row, i) => (
          <div
            key={row.key}
            className="flex flex-col gap-3 rounded-2xl border border-zinc-200 p-4 dark:border-zinc-800"
          >
            <div className="flex items-center justify-between">
              <span className="text-sm font-medium">{i + 1}. időpont</span>
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
        {rows.length < MAX_IDOPONTOK ? (
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

      {state.error ? (
        <p role="alert" className="text-sm text-red-600 dark:text-red-400">
          {state.error}
        </p>
      ) : null}
      <button
        type="submit"
        disabled={pending}
        className={`${buttonClass} self-start`}
      >
        {pending ? "Létrehozás…" : "Esemény létrehozása"}
      </button>
    </form>
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

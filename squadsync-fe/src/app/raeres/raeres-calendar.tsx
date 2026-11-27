"use client";

import { useState, useTransition } from "react";
import type { RaeresAllapot } from "@/lib/backend";
import {
  daysBetween,
  daysInMonth,
  firstOfMonth,
  formatDay,
  formatMonth,
  formatShortDay,
  nextMonth,
  weekdayMondayFirst,
} from "@/lib/raeres-days";
import { secondaryButtonClass } from "@/lib/ui";
import { type SetRaeresState, setRaeresAction } from "./actions";

type Marks = Record<string, RaeresAllapot>;
type Mode = "egy-nap" | "tartomany";

const weekdayHeads = ["H", "K", "Sze", "Cs", "P", "Szo", "V"];

const allapotLabel: Record<RaeresAllapot | "ismeretlen", string> = {
  szabad: "szabad",
  foglalt: "foglalt",
  ismeretlen: "nincs megjelölve",
};

// Unknown, then szabad, then foglalt, then unknown again.
function nextAllapot(current: RaeresAllapot | undefined): RaeresAllapot | "" {
  if (current === undefined) {
    return "szabad";
  }
  return current === "szabad" ? "foglalt" : "";
}

/**
 * The calendar of the caller's own Ráérés, month by month from today to the
 * end of the window. In "Egy nap" mode a tap cycles the day through unknown,
 * szabad and foglalt. In "Tartomány" mode two taps pick the first and last
 * day of a range, and a button then marks the whole range at once. Changes
 * show at once and are saved through a server action; a refused save is
 * taken back.
 */
export function RaeresCalendar({
  today,
  lastDay,
  initial,
}: {
  today: string;
  lastDay: string;
  initial: Marks;
}) {
  const [marks, setMarks] = useState<Marks>(initial);
  const [mode, setMode] = useState<Mode>("egy-nap");
  const [range, setRange] = useState<{ start: string; end: string | null }>();
  const [error, setError] = useState<string>();
  const [pending, startTransition] = useTransition();

  const months: string[] = [];
  for (let m = firstOfMonth(today); m <= lastDay; m = nextMonth(m)) {
    months.push(m);
  }

  const selection = range
    ? ([range.start, range.end ?? range.start].sort() as [string, string])
    : null;

  // Marks the days, shows it at once, and puts the old marks back if the
  // save is refused.
  function paint(from: string, to: string, allapot: RaeresAllapot | "") {
    const days = daysBetween(from, to);
    const before = marks;
    setError(undefined);
    setMarks((current) => {
      const next = { ...current };
      for (const day of days) {
        if (allapot === "") {
          delete next[day];
        } else {
          next[day] = allapot;
        }
      }
      return next;
    });
    startTransition(async () => {
      let result: SetRaeresState;
      try {
        result = await setRaeresAction(from, to, allapot);
      } catch {
        result = { error: "Nem sikerült menteni. Próbáld újra." };
      }
      if (result.error) {
        setError(result.error);
        // Take back only the days still showing this failed change, so a
        // later tap on the same day isn't undone with it.
        const optimistic = allapot === "" ? undefined : allapot;
        setMarks((current) => {
          const next = { ...current };
          for (const day of days) {
            if (current[day] !== optimistic) {
              continue;
            }
            if (before[day] === undefined) {
              delete next[day];
            } else {
              next[day] = before[day];
            }
          }
          return next;
        });
      }
    });
  }

  function tap(day: string) {
    if (mode === "egy-nap") {
      paint(day, day, nextAllapot(marks[day]));
      return;
    }
    if (!range || range.end !== null) {
      setRange({ start: day, end: null });
    } else {
      setRange({ start: range.start, end: day });
    }
  }

  function applyToRange(allapot: RaeresAllapot | "") {
    if (!selection) {
      return;
    }
    paint(selection[0], selection[1], allapot);
    setRange(undefined);
  }

  function chooseMode(next: Mode) {
    setMode(next);
    setRange(undefined);
  }

  return (
    <div
      className={`flex flex-col gap-6 ${mode === "tartomany" ? "pb-40" : ""}`}
    >
      <div className="flex flex-col gap-3">
        <fieldset className="flex gap-2">
          <legend className="sr-only">Jelölés módja</legend>
          {(
            [
              ["egy-nap", "Egy nap"],
              ["tartomany", "Tartomány"],
            ] as const
          ).map(([value, label]) => (
            <button
              key={value}
              type="button"
              aria-pressed={mode === value}
              onClick={() => chooseMode(value)}
              className={`rounded-full border px-4 py-2 text-sm font-medium ${
                mode === value
                  ? "border-foreground bg-foreground text-background"
                  : "border-zinc-300 dark:border-zinc-700"
              }`}
            >
              {label}
            </button>
          ))}
        </fieldset>
        <p className="text-sm text-zinc-600 dark:text-zinc-400">
          {mode === "egy-nap"
            ? "Koppints egy napra: nincs megjelölve, szabad, foglalt, majd újra nincs megjelölve."
            : "Koppints a tartomány első, majd utolsó napjára, aztán válaszd ki, mire állítod az egészet."}
        </p>
        <ul className="flex flex-wrap gap-x-4 gap-y-1 text-sm">
          <li className="flex items-center gap-2">
            <span className={`${swatchClass} ${dayClass.szabad}`}>✓</span>
            szabad
          </li>
          <li className="flex items-center gap-2">
            <span className={`${swatchClass} ${dayClass.foglalt}`}>✕</span>
            foglalt
          </li>
          <li className="flex items-center gap-2">
            <span className={`${swatchClass} ${dayClass.ismeretlen}`} />
            nincs megjelölve
          </li>
        </ul>
        <output className="block min-h-5 text-sm">
          {pending ? <span className="text-zinc-500">Mentés…</span> : null}
          {error ? (
            <span role="alert" className="text-red-600 dark:text-red-400">
              {error}
            </span>
          ) : null}
        </output>
      </div>

      <div className="grid gap-8 sm:grid-cols-2">
        {months.map((month) => (
          <Month
            key={month}
            month={month}
            today={today}
            lastDay={lastDay}
            marks={marks}
            selection={selection}
            onTap={tap}
          />
        ))}
      </div>

      {mode === "tartomany" ? (
        <div className="sticky bottom-4 flex flex-col gap-3 rounded-2xl border border-zinc-200 bg-background p-4 shadow-lg dark:border-zinc-800">
          <p className="text-sm">
            {!selection
              ? "Válaszd ki a tartomány első napját."
              : range?.end === null
                ? `Első nap: ${formatShortDay(selection[0])}. Válaszd ki az utolsó napot (vagy állítsd csak ezt az egyet).`
                : `${formatShortDay(selection[0])} – ${formatShortDay(selection[1])} (${daysBetween(selection[0], selection[1]).length} nap)`}
          </p>
          <div className="flex flex-wrap gap-2">
            <button
              type="button"
              disabled={!selection}
              onClick={() => applyToRange("szabad")}
              className={`${secondaryButtonClass} disabled:opacity-40`}
            >
              Szabad
            </button>
            <button
              type="button"
              disabled={!selection}
              onClick={() => applyToRange("foglalt")}
              className={`${secondaryButtonClass} disabled:opacity-40`}
            >
              Foglalt
            </button>
            <button
              type="button"
              disabled={!selection}
              onClick={() => applyToRange("")}
              className={`${secondaryButtonClass} disabled:opacity-40`}
            >
              Jelölés törlése
            </button>
            <button
              type="button"
              disabled={!selection}
              onClick={() => setRange(undefined)}
              className={`${secondaryButtonClass} disabled:opacity-40`}
            >
              Mégse
            </button>
          </div>
        </div>
      ) : null}
    </div>
  );
}

const swatchClass =
  "inline-flex size-6 items-center justify-center rounded-md text-xs";

const dayClass: Record<RaeresAllapot | "ismeretlen", string> = {
  szabad: "bg-green-700 text-white",
  foglalt: "bg-red-600 text-white",
  ismeretlen: "bg-zinc-100 dark:bg-zinc-900",
};

function Month({
  month,
  today,
  lastDay,
  marks,
  selection,
  onTap,
}: {
  month: string;
  today: string;
  lastDay: string;
  marks: Marks;
  selection: [string, string] | null;
  onTap: (day: string) => void;
}) {
  const blanks = weekdayMondayFirst(month);
  const count = daysInMonth(month);
  const cells: (string | null)[] = Array.from({ length: blanks }, () => null);
  for (let d = 1; d <= count; d++) {
    cells.push(`${month.slice(0, 8)}${String(d).padStart(2, "0")}`);
  }

  return (
    <section className="flex flex-col gap-2">
      <h2 className="text-lg font-semibold capitalize">{formatMonth(month)}</h2>
      <div className="grid grid-cols-7 gap-1 text-center text-xs text-zinc-500">
        {weekdayHeads.map((head) => (
          <span key={head}>{head}</span>
        ))}
      </div>
      <div className="grid grid-cols-7 gap-1">
        {cells.map((day, i) => {
          const key = day ?? `${month}-blank-${i}`;
          // Past days and days beyond the window can't be marked, and aren't shown.
          if (day === null || day < today || day > lastDay) {
            return <span key={key} />;
          }
          const allapot = marks[day];
          const selected =
            selection !== null && day >= selection[0] && day <= selection[1];
          return (
            <button
              key={key}
              type="button"
              onClick={() => onTap(day)}
              aria-label={`${formatDay(day)}: ${allapotLabel[allapot ?? "ismeretlen"]}`}
              aria-pressed={selected || undefined}
              className={`flex aspect-square touch-manipulation select-none flex-col items-center justify-center rounded-lg text-sm leading-none ${dayClass[allapot ?? "ismeretlen"]} ${
                day === today ? "font-bold underline" : ""
              } ${selected ? "ring-2 ring-blue-500 ring-offset-1" : ""}`}
            >
              <span>{Number(day.slice(8))}</span>
              <span className="mt-0.5 h-3 text-[10px]" aria-hidden="true">
                {allapot === "szabad" ? "✓" : allapot === "foglalt" ? "✕" : ""}
              </span>
            </button>
          );
        })}
      </div>
    </section>
  );
}

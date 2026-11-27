"use client";

import { useState } from "react";
import {
  daysInMonth,
  firstOfMonth,
  formatDay,
  formatMonth,
  nextMonth,
  weekdayMondayFirst,
} from "@/lib/raeres-days";
import { secondaryButtonClass } from "@/lib/ui";

const weekdayHeads = ["H", "K", "Sze", "Cs", "P", "Szo", "V"];

// A day is shaded by the share of the Csoport's Tags who are szabad.
function shade(szabad: number, osszes: number): string {
  if (osszes === 0 || szabad === 0) {
    return "bg-zinc-100 dark:bg-zinc-900";
  }
  const share = szabad / osszes;
  if (share >= 1) {
    return "bg-green-700 text-white dark:bg-green-500 dark:text-black";
  }
  if (share > 0.75) {
    return "bg-green-600 text-white dark:bg-green-600";
  }
  if (share > 0.5) {
    return "bg-green-400 dark:bg-green-700";
  }
  if (share > 0.25) {
    return "bg-green-200 dark:bg-green-900";
  }
  return "bg-green-100 dark:bg-green-950";
}

/**
 * The Csoport's heatmap, one month at a time from the current month to the
 * end of the window. The counts arrive for the whole window at once, so
 * paging needs no further requests.
 */
export function HeatmapMonths({
  today,
  lastDay,
  counts,
}: {
  today: string;
  lastDay: string;
  /** Each day's [szabad Tags, all Tags]. */
  counts: Record<string, [number, number]>;
}) {
  const months: string[] = [];
  for (let m = firstOfMonth(today); m <= lastDay; m = nextMonth(m)) {
    months.push(m);
  }
  const [index, setIndex] = useState(0);
  const month = months[index];

  const cells: (string | null)[] = Array.from(
    { length: weekdayMondayFirst(month) },
    () => null,
  );
  for (let d = 1; d <= daysInMonth(month); d++) {
    cells.push(`${month.slice(0, 8)}${String(d).padStart(2, "0")}`);
  }

  return (
    <div className="flex flex-col gap-3">
      <div className="flex items-center justify-between gap-2">
        <button
          type="button"
          disabled={index === 0}
          onClick={() => setIndex(index - 1)}
          className={`${secondaryButtonClass} disabled:opacity-40`}
        >
          ← Előző
        </button>
        <h3 className="font-semibold capitalize">{formatMonth(month)}</h3>
        <button
          type="button"
          disabled={index === months.length - 1}
          onClick={() => setIndex(index + 1)}
          className={`${secondaryButtonClass} disabled:opacity-40`}
        >
          Továbbiak →
        </button>
      </div>
      <div className="grid grid-cols-7 gap-1 text-center text-xs">
        {weekdayHeads.map((head) => (
          <span key={head} className="text-zinc-500">
            {head}
          </span>
        ))}
        {cells.map((day, i) => {
          const key = day ?? `${month}-blank-${i}`;
          const entry = day === null ? undefined : counts[day];
          // Past days and days beyond the window aren't shown.
          if (day === null || day < today || day > lastDay || !entry) {
            return <span key={key} />;
          }
          const [szabad, osszes] = entry;
          const text = `${formatDay(day)}: ${szabad}/${osszes} tag szabad`;
          return (
            <span
              key={key}
              title={text}
              role="img"
              aria-label={text}
              className={`flex aspect-square flex-col items-center justify-center rounded-md leading-none ${shade(szabad, osszes)} ${
                day === today ? "ring-2 ring-foreground" : ""
              }`}
            >
              <span className="text-[10px] opacity-80">
                {Number(day.slice(8))}
              </span>
              <span className="text-sm font-semibold">{szabad}</span>
            </span>
          );
        })}
      </div>
    </div>
  );
}

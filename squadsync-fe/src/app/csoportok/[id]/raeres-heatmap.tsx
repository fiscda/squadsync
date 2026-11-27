import Link from "next/link";
import type { HeatmapDay } from "@/lib/backend";
import { requireFelhasznalo } from "@/lib/felhasznalo";
import { getHeatmap } from "@/lib/raeres";
import {
  addDays,
  budapestToday,
  daysBetween,
  formatDay,
  formatShortDay,
  weekdayMondayFirst,
} from "@/lib/raeres-days";
import { linkClass } from "@/lib/ui";

const WEEKS = 8;

const weekdayHeads = ["H", "K", "Sze", "Cs", "P", "Szo", "V"];

// A day is shaded by the share of the Csoport's Tags who are szabad.
function shade(day: HeatmapDay): string {
  if (day.osszes === 0 || day.szabad === 0) {
    return "bg-zinc-100 dark:bg-zinc-900";
  }
  const share = day.szabad / day.osszes;
  if (share >= 1) {
    return "bg-green-700 text-white dark:bg-green-500 dark:text-black";
  }
  if (share > 0.75) {
    return "bg-green-500 text-white dark:bg-green-600";
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
 * How many of the Csoport's Tags are free on each of the next eight weeks'
 * days. Only counts, never who: Ráérés is shared as numbers.
 */
export async function RaeresHeatmap({ csoportId }: { csoportId: string }) {
  // Reads the session first: only after that may the page ask for today's date.
  await requireFelhasznalo();
  const today = budapestToday();
  const last = addDays(today, WEEKS * 7 - 1);
  const days = (await getHeatmap(csoportId, today, last)) ?? [];
  const byDay = new Map(days.map((d) => [d.day, d]));

  // Rows are weeks, Monday to Sunday; the days before today stay blank.
  const firstMonday = addDays(today, -weekdayMondayFirst(today));
  const weeks: string[][] = [];
  for (let start = firstMonday; start <= last; start = addDays(start, 7)) {
    weeks.push(daysBetween(start, addDays(start, 6)));
  }

  return (
    <section className="flex flex-col gap-3">
      <div className="flex items-center justify-between gap-4">
        <h2 className="text-xl font-semibold">Ráérés</h2>
        <Link href="/raeres" className={`${linkClass} text-sm`}>
          Ráérésem kitöltése
        </Link>
      </div>
      <p className="text-sm text-zinc-600 dark:text-zinc-400">
        A következő {WEEKS} hét: hány tag ér rá az adott napon. Minél sötétebb,
        annál többen.
      </p>
      <div className="grid grid-cols-[auto_repeat(7,minmax(0,1fr))] gap-1 text-center text-xs">
        <span />
        {weekdayHeads.map((head) => (
          <span key={head} className="text-zinc-500">
            {head}
          </span>
        ))}
        {weeks.map((week) => (
          <WeekRow
            key={week[0]}
            week={week}
            today={today}
            last={last}
            byDay={byDay}
          />
        ))}
      </div>
    </section>
  );
}

function WeekRow({
  week,
  today,
  last,
  byDay,
}: {
  week: string[];
  today: string;
  last: string;
  byDay: Map<string, HeatmapDay>;
}) {
  return (
    <>
      <span className="flex items-center pr-1 text-zinc-500">
        {formatShortDay(week[0])}
      </span>
      {week.map((day) => {
        const entry = byDay.get(day);
        if (day < today || day > last || !entry) {
          return <span key={day} />;
        }
        const text = `${formatDay(day)}: ${entry.szabad}/${entry.osszes} tag szabad`;
        return (
          <span
            key={day}
            title={text}
            role="img"
            aria-label={text}
            className={`flex aspect-square flex-col items-center justify-center rounded-md leading-none ${shade(entry)} ${
              day === today ? "ring-2 ring-foreground" : ""
            }`}
          >
            <span className="text-[10px] opacity-80">
              {Number(day.slice(8))}
            </span>
            <span className="text-sm font-semibold">{entry.szabad}</span>
          </span>
        );
      })}
    </>
  );
}

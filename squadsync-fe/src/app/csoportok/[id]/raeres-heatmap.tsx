import Link from "next/link";
import { requireFelhasznalo } from "@/lib/felhasznalo";
import { getHeatmap } from "@/lib/raeres";
import { addDays, budapestToday, RAERES_WINDOW_DAYS } from "@/lib/raeres-days";
import { linkClass } from "@/lib/ui";
import { HeatmapMonths } from "./heatmap-months";

/**
 * How many of the Csoport's Tags are free on each day of the whole window,
 * shown month by month. Only counts, never who: Ráérés is shared as numbers.
 */
export async function RaeresHeatmap({ csoportId }: { csoportId: string }) {
  // Reads the session first: only after that may the page ask for today's date.
  await requireFelhasznalo();
  const today = budapestToday();
  const lastDay = addDays(today, RAERES_WINDOW_DAYS);
  const days = (await getHeatmap(csoportId, today, lastDay)) ?? [];

  return (
    <section className="flex flex-col gap-3">
      <div className="flex items-center justify-between gap-4">
        <h2 className="text-xl font-semibold">Ráérés</h2>
        <Link href="/raeres" className={`${linkClass} text-sm`}>
          Ráérésem kitöltése
        </Link>
      </div>
      <p className="text-sm text-zinc-600 dark:text-zinc-400">
        Hány tag ér rá az adott napon. Minél sötétebb, annál többen.
      </p>
      <HeatmapMonths
        today={today}
        lastDay={lastDay}
        counts={Object.fromEntries(
          days.map((d) => [d.day, [d.szabad, d.osszes]]),
        )}
      />
    </section>
  );
}

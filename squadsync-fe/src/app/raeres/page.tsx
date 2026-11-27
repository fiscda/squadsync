import Link from "next/link";
import { Suspense } from "react";
import { requireFelhasznalo } from "@/lib/felhasznalo";
import { getMyRaeres } from "@/lib/raeres";
import { addDays, budapestToday, RAERES_WINDOW_DAYS } from "@/lib/raeres-days";
import { linkClass } from "@/lib/ui";
import { RaeresCalendar } from "./raeres-calendar";

export default function RaeresPage() {
  return (
    <main className="mx-auto flex w-full max-w-2xl flex-1 flex-col gap-6 px-4 py-12">
      <div className="flex flex-col gap-2">
        <Link href="/" className={`${linkClass} text-sm`}>
          ← Főoldal
        </Link>
        <h1 className="text-3xl font-semibold tracking-tight">Ráérésem</h1>
        <p className="text-sm text-zinc-600 dark:text-zinc-400">
          Jelöld meg, mely napokon érsz rá. A Ráérésed minden csoportodban
          ugyanaz, és csak a csoporttársaid látják, hogy hányan érnek rá egy
          napon. Nem látják, ki mikor, és az okot sem.
        </p>
      </div>
      <Suspense fallback={<p className="text-zinc-500">Betöltés…</p>}>
        <Calendar />
      </Suspense>
    </main>
  );
}

// Reads the session first: only after that may the page ask for today's date.
async function Calendar() {
  await requireFelhasznalo();
  const today = budapestToday();
  const lastDay = addDays(today, RAERES_WINDOW_DAYS);
  const marked = await getMyRaeres(today, lastDay);

  return (
    <RaeresCalendar
      today={today}
      lastDay={lastDay}
      initial={Object.fromEntries(marked.map((d) => [d.day, d.allapot]))}
    />
  );
}

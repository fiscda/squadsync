import type { Allapot, Valasz } from "@/lib/backend";

// Times arrive in UTC and are always shown in Budapest time.

const dateTimeFormat = new Intl.DateTimeFormat("hu-HU", {
  year: "numeric",
  month: "long",
  day: "numeric",
  weekday: "long",
  hour: "2-digit",
  minute: "2-digit",
  timeZone: "Europe/Budapest",
});

const dateFormat = new Intl.DateTimeFormat("hu-HU", {
  year: "numeric",
  month: "short",
  day: "numeric",
  weekday: "short",
  timeZone: "Europe/Budapest",
});

/**
 * A whole-day Időpont as its days, e.g. "2026. dec. 5., szo. – dec. 7., h.".
 * Its end is midnight after the last day, so step back to that day.
 */
export function formatEgeszNapos(start: string, end: string): string {
  const lastDay = new Date(new Date(end).getTime() - 1);
  return dateFormat.formatRange(new Date(start), lastDay);
}

/**
 * An Időpont as Budapest local time, e.g. "2026. december 5., szombat
 * 20:00–23:00", or a range across days for a multi-day one.
 */
export function formatIdopont(start: string, end: string | null): string {
  if (end === null) {
    return dateTimeFormat.format(new Date(start));
  }
  return dateTimeFormat.formatRange(new Date(start), new Date(end));
}

export const allapotLabel: Record<Allapot, string> = {
  egyeztetes: "Egyeztetés",
  veglegesitve: "Véglegesítve",
  elmaradt: "Elmaradt",
  lezajlott: "Lezajlott",
};

export const valaszLabel: Record<Valasz, string> = {
  igen: "Igen",
  talan: "Talán",
  nem: "Nem",
};

// Calendar-day arithmetic for Ráérés. A day is a Budapest date as text, like
// "2026-12-05"; the same text works as a sort key. No time zones are involved
// except in budapestToday: days are plain dates, handled as midnight UTC.
// Safe to import from client components.

/** How many days after today can still be marked (matches Go's RaeresWindowDays). */
export const RAERES_WINDOW_DAYS = 365;

const budapestDate = new Intl.DateTimeFormat("en-CA", {
  year: "numeric",
  month: "2-digit",
  day: "2-digit",
  timeZone: "Europe/Budapest",
});

/** Today's date in Budapest. */
export function budapestToday(now: Date = new Date()): string {
  return budapestDate.format(now);
}

function toDate(day: string): Date {
  return new Date(`${day}T00:00:00Z`);
}

function fromDate(date: Date): string {
  return date.toISOString().slice(0, 10);
}

/** The day n days after (or before, if negative) the given one. */
export function addDays(day: string, n: number): string {
  const date = toDate(day);
  date.setUTCDate(date.getUTCDate() + n);
  return fromDate(date);
}

/** Monday is 0 and Sunday is 6. */
export function weekdayMondayFirst(day: string): number {
  return (toDate(day).getUTCDay() + 6) % 7;
}

/** The first day of the month the day is in. */
export function firstOfMonth(day: string): string {
  return `${day.slice(0, 7)}-01`;
}

/** The first day of the following month. */
export function nextMonth(day: string): string {
  const date = toDate(firstOfMonth(day));
  date.setUTCMonth(date.getUTCMonth() + 1);
  return fromDate(date);
}

/** How many days the month of the given day has. */
export function daysInMonth(day: string): number {
  const date = toDate(nextMonth(day));
  date.setUTCDate(0);
  return date.getUTCDate();
}

/** Every day from..to, both included. Empty if to is before from. */
export function daysBetween(from: string, to: string): string[] {
  const days: string[] = [];
  for (let day = from; day <= to; day = addDays(day, 1)) {
    days.push(day);
  }
  return days;
}

const dayLabelFormat = new Intl.DateTimeFormat("hu-HU", {
  month: "long",
  day: "numeric",
  weekday: "long",
  timeZone: "UTC",
});

const monthLabelFormat = new Intl.DateTimeFormat("hu-HU", {
  year: "numeric",
  month: "long",
  timeZone: "UTC",
});

const shortDayFormat = new Intl.DateTimeFormat("hu-HU", {
  month: "short",
  day: "numeric",
  timeZone: "UTC",
});

/** "március 10., kedd" */
export function formatDay(day: string): string {
  return dayLabelFormat.format(toDate(day));
}

/** "2026. március" */
export function formatMonth(day: string): string {
  return monthLabelFormat.format(toDate(day));
}

/** "márc. 10." */
export function formatShortDay(day: string): string {
  return shortDayFormat.format(toDate(day));
}

// Tailwind classes shared by the pages.

export const buttonClass =
  "rounded-full bg-foreground px-5 py-3 font-medium text-background transition-opacity hover:opacity-80";

export const secondaryButtonClass =
  "rounded-full border border-zinc-300 px-4 py-2 text-sm font-medium transition-colors hover:bg-zinc-100 dark:border-zinc-700 dark:hover:bg-zinc-900";

export const linkClass = "underline underline-offset-4 hover:opacity-80";

export const cardClass =
  "rounded-2xl border border-zinc-200 p-4 dark:border-zinc-800";

/** A date like "2026. március 2." in the group's time zone. */
export function formatDate(iso: string): string {
  return new Intl.DateTimeFormat("hu-HU", {
    dateStyle: "long",
    timeZone: "Europe/Budapest",
  }).format(new Date(iso));
}

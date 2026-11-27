// The Emlékeztető offsets a Csoport can choose from, in minutes before the
// start, longest first. Kept in sync by hand with app.EmlekeztetoValasztek in
// Go, which has the final say.
export const EMLEKEZTETO_VALASZTEK: { perc: number; label: string }[] = [
  { perc: 7 * 24 * 60, label: "1 héttel előtte" },
  { perc: 3 * 24 * 60, label: "3 nappal előtte" },
  { perc: 24 * 60, label: "1 nappal előtte" },
  { perc: 3 * 60, label: "3 órával előtte" },
  { perc: 60, label: "1 órával előtte" },
  { perc: 15, label: "15 perccel előtte" },
];

/** An offset as shown, e.g. "1 nappal előtte"; unknown ones in minutes. */
export function emlekeztetoLabel(perc: number): string {
  return (
    EMLEKEZTETO_VALASZTEK.find((v) => v.perc === perc)?.label ??
    `${perc} perccel előtte`
  );
}

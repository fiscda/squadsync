const ft = new Intl.NumberFormat("hu-HU", { maximumFractionDigits: 0 });

/** Whole forints with Hungarian grouping: "12 000 Ft". */
export function formatFt(amount: number): string {
  return `${ft.format(amount)} Ft`;
}

/** A balance with its sign: "+500 Ft", "−500 Ft", "0 Ft". */
export function formatNetto(amount: number): string {
  if (amount > 0) {
    return `+${formatFt(amount)}`;
  }
  if (amount < 0) {
    return `−${formatFt(-amount)}`;
  }
  return formatFt(0);
}

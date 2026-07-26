// Miles are always shown with pt-BR thousands separators, never abbreviated: an operator
// reconciling a balance needs the exact figure, and "1,6 mi" hides the last five digits.
// Pair it with `tabular-nums` wherever the numbers stack in a column.
export function formatPoints(points: number): string {
  return points.toLocaleString("pt-BR")
}

// Compact form for chart axis ticks and stat tiles, where the exact digits belong in the
// tooltip or the table view rather than crowding the axis.
export function formatPointsCompact(points: number): string {
  return points.toLocaleString("pt-BR", {
    notation: "compact",
    maximumFractionDigits: 1,
  })
}

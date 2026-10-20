// A count of things - issuances, cancellations - with pt-BR thousands separators. Not for
// points: those always go through formatSignedPaddedPoints (lib/points), which pads 500 to
// "0.500"; a count of 500 issuances is just "500".
export function formatCount(count: number): string {
  return count.toLocaleString("pt-BR")
}

// Compact form for chart axis ticks and stat tiles, where the exact digits belong in the
// tooltip or the table view rather than crowding the axis.
export function formatPointsCompact(points: number): string {
  return points.toLocaleString("pt-BR", {
    notation: "compact",
    maximumFractionDigits: 1,
  })
}

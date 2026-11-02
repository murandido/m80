import { formatCurrencyBRL } from "@/lib/currency"
import { formatPointsCompact } from "@/lib/format"
import { formatSignedPaddedPoints } from "@/lib/points"
import type { DashboardMetric } from "@/app/dashboard/dashboard-params"

// The eight categorical slots defined in globals.css. The ORDER is the colourblind-safety
// mechanism, not a style choice - neighbouring slots were validated as distinguishable
// under protanopia and deuteranopia - so entities take slots in sequence and the list is
// never cycled. Past the eighth, callers fold the tail into "Outros" rather than inventing
// a ninth hue, which would be indistinguishable from an existing one.
export const CHART_SLOTS = [
  "var(--chart-1)",
  "var(--chart-2)",
  "var(--chart-3)",
  "var(--chart-4)",
  "var(--chart-5)",
  "var(--chart-6)",
  "var(--chart-7)",
  "var(--chart-8)",
] as const

export const MAX_CHART_SLICES = CHART_SLOTS.length

export function chartSlot(index: number): string {
  return CHART_SLOTS[index] ?? CHART_SLOTS[CHART_SLOTS.length - 1]
}

// Reads the metric off a row. Returns 0 when the server withheld amount_cents, which only
// happens for non-admins - and they can never select the "amount" metric in the first
// place, so this fallback is a type guard rather than a display path.
export function metricValue(
  row: { points: number; amount_cents?: number },
  metric: DashboardMetric,
): number {
  return metric === "amount" ? (row.amount_cents ?? 0) : row.points
}

export function formatMetric(value: number, metric: DashboardMetric): string {
  return metric === "amount" ? formatCurrencyBRL(value) : formatSignedPaddedPoints(value)
}

// Axis ticks and stat-tile values get the compact form; the exact digits stay reachable in
// the tooltip and the table view, so nothing is gated by abbreviating here.
export function formatMetricCompact(value: number, metric: DashboardMetric): string {
  if (metric !== "amount") return formatPointsCompact(value)
  return (value / 100).toLocaleString("pt-BR", {
    style: "currency",
    currency: "BRL",
    notation: "compact",
    maximumFractionDigits: 1,
  })
}

export function metricLabel(metric: DashboardMetric): string {
  return metric === "amount" ? "Valor de venda" : "Pontos"
}

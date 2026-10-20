import { formatCurrencyBRL } from "@/lib/currency"
import { formatCount } from "@/lib/format"
import { formatSignedPaddedPoints } from "@/lib/points"
import type { Dashboard } from "@/schemas/dashboard"
import { StatTile } from "./stat-tile"

interface DashboardStatsProps {
  dashboard: Dashboard
  canSeeAmounts: boolean
}

// The KPI row. Each figure is a stat tile rather than a one-bar chart, and each carries a
// delta against the same window shifted back - the number alone says how much, the delta
// says whether that is unusual.
export function DashboardStats({ dashboard, canSeeAmounts }: DashboardStatsProps) {
  const { totals, previous_totals: previous } = dashboard

  const averagePoints = average(totals.points, totals.issuance_count)
  const previousAveragePoints = previous
    ? average(previous.points, previous.issuance_count)
    : undefined

  return (
    <div className="grid gap-3 sm:grid-cols-2 xl:grid-cols-4">
      <StatTile
        label="Pontos emitidos"
        value={formatSignedPaddedPoints(totals.points)}
        current={totals.points}
        previous={previous?.points}
        previousValue={previous ? formatSignedPaddedPoints(previous.points) : undefined}
      />

      <StatTile
        label="Emissões"
        value={formatCount(totals.issuance_count)}
        current={totals.issuance_count}
        previous={previous?.issuance_count}
        previousValue={previous ? formatCount(previous.issuance_count) : undefined}
      />

      <StatTile
        label="Média por emissão"
        value={formatSignedPaddedPoints(averagePoints)}
        current={averagePoints}
        previous={previousAveragePoints}
        previousValue={
          previousAveragePoints === undefined
            ? undefined
            : formatSignedPaddedPoints(previousAveragePoints)
        }
      />

      {/* The fourth tile is the money for an admin and the cancellations for everyone
          else, so the row stays balanced instead of leaving a hole. */}
      {canSeeAmounts && totals.amount_cents !== undefined ? (
        <StatTile
          label="Valor de venda"
          value={formatCurrencyBRL(totals.amount_cents)}
          current={totals.amount_cents}
          previous={previous?.amount_cents}
          previousValue={
            previous?.amount_cents === undefined
              ? undefined
              : formatCurrencyBRL(previous.amount_cents)
          }
        />
      ) : (
        <StatTile
          label="Cancelamentos"
          value={formatCount(totals.cancelled_count)}
          current={totals.cancelled_count}
          previous={previous?.cancelled_count}
          previousValue={
            previous ? formatCount(previous.cancelled_count) : undefined
          }
          upIsGood={false}
        />
      )}
    </div>
  )
}

function average(total: number, count: number): number {
  if (count === 0) return 0
  return Math.round(total / count)
}

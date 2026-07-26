"use client"

import { useMemo } from "react"
import { Bar, BarChart, LabelList, XAxis, YAxis } from "recharts"
import {
  ChartContainer,
  ChartTooltip,
  ChartTooltipContent,
  type ChartConfig,
} from "@/components/ui/chart"
import type { DashboardBreakdown } from "@/schemas/dashboard"
import type { DashboardMetric } from "@/app/dashboard/dashboard-params"
import { ChartCard } from "./chart-card"
import { formatMetric, formatMetricCompact, metricLabel, metricValue } from "./metric"

interface PointsByIssuerChartProps {
  byIssuer: DashboardBreakdown[]
  metric: DashboardMetric
}

const BAR_HEIGHT = 34
// Enough to see the team without the card growing without bound; the rest stay in the
// table view, which lists everyone.
const MAX_VISIBLE_ISSUERS = 8

// One colour for every bar, on purpose. This is a pure magnitude ranking with the names
// already on the axis, so painting each person a different hue would spend the identity
// channel re-encoding what the bar length already says - and would imply a grouping that
// does not exist.
export function PointsByIssuerChart({ byIssuer, metric }: PointsByIssuerChartProps) {
  const all = useMemo(
    () =>
      byIssuer
        .map((issuer) => ({
          id: issuer.id,
          name: issuer.name,
          value: metricValue(issuer, metric),
          issuanceCount: issuer.issuance_count,
        }))
        .filter((issuer) => issuer.value > 0)
        .sort((a, b) => b.value - a.value),
    [byIssuer, metric],
  )

  const visible = all.slice(0, MAX_VISIBLE_ISSUERS)

  const config = {
    value: { label: metricLabel(metric), color: "var(--chart-1)" },
  } satisfies ChartConfig

  const rows = all.map((issuer) => ({
    key: issuer.id,
    cells: [
      issuer.name,
      String(issuer.issuanceCount),
      formatMetric(issuer.value, metric),
    ],
  }))

  return (
    <ChartCard
      title="Por funcionário"
      description={
        all.length > visible.length
          ? `${metricLabel(metric)} — top ${visible.length} de ${all.length}`
          : metricLabel(metric)
      }
      tableHeaders={["Funcionário", "Emissões", metricLabel(metric)]}
      tableRows={rows}
      isEmpty={visible.length === 0}
    >
      <ChartContainer
        config={config}
        className="aspect-auto w-full"
        style={{ height: Math.max(visible.length * BAR_HEIGHT + 24, 120) }}
      >
        <BarChart
          data={visible}
          layout="vertical"
          margin={{ left: 4, right: 64, top: 4, bottom: 4 }}
        >
          <XAxis type="number" dataKey="value" hide />
          <YAxis
            type="category"
            dataKey="name"
            tickLine={false}
            axisLine={false}
            width={128}
            tickMargin={4}
          />

          <ChartTooltip
            cursor={{ fill: "var(--muted)" }}
            content={
              <ChartTooltipContent
                formatter={(value) => formatMetric(Number(value), metric)}
              />
            }
          />

          <Bar
            dataKey="value"
            name={metricLabel(metric)}
            fill="var(--color-value)"
            radius={[0, 4, 4, 0]}
            maxBarSize={24}
          >
            <LabelList
              dataKey="value"
              position="right"
              className="fill-slate-600"
              fontSize={12}
              formatter={(value: unknown) => formatMetricCompact(Number(value), metric)}
            />
          </Bar>
        </BarChart>
      </ChartContainer>
    </ChartCard>
  )
}

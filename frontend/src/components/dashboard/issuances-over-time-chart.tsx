"use client"

import { useMemo } from "react"
import { Area, AreaChart, CartesianGrid, XAxis, YAxis } from "recharts"
import {
  ChartContainer,
  ChartTooltip,
  ChartTooltipContent,
  type ChartConfig,
} from "@/components/ui/chart"
import type { DashboardBucket } from "@/schemas/dashboard"
import type { DashboardMetric } from "@/app/dashboard/dashboard-params"
import { ChartCard } from "./chart-card"
import { formatBucketLabel } from "./bucket-label"
import { formatMetric, formatMetricCompact, metricLabel, metricValue } from "./metric"

interface IssuancesOverTimeChartProps {
  series: DashboardBucket[]
  metric: DashboardMetric
  granularity: string
}

// One series, so no legend box: the card title already says what is plotted. An area (a
// 10% wash under a 2px line) rather than bars, because the question here is the shape of
// the period, not a per-bucket comparison.
export function IssuancesOverTimeChart({
  series,
  metric,
  granularity,
}: IssuancesOverTimeChartProps) {
  const data = useMemo(
    () =>
      series.map((bucket) => ({
        bucket: bucket.bucket,
        label: formatBucketLabel(bucket.bucket, granularity),
        value: metricValue(bucket, metric),
        issuanceCount: bucket.issuance_count,
      })),
    [series, metric, granularity],
  )

  const config = {
    value: { label: metricLabel(metric), color: "var(--chart-1)" },
  } satisfies ChartConfig

  const rows = data.map((point) => ({
    key: point.bucket,
    cells: [
      point.label,
      String(point.issuanceCount),
      formatMetric(point.value, metric),
    ],
  }))

  return (
    <ChartCard
      title="Emissões ao longo do período"
      description={`${metricLabel(metric)} por ${GRANULARITY_NOUN[granularity] ?? "período"}`}
      tableHeaders={["Período", "Emissões", metricLabel(metric)]}
      tableRows={rows}
      isEmpty={data.every((point) => point.value === 0)}
    >
      <ChartContainer config={config} className="aspect-auto h-64 w-full">
        <AreaChart data={data} margin={{ left: 4, right: 12, top: 8, bottom: 0 }}>
          <defs>
            <linearGradient id="issuancesOverTimeFill" x1="0" y1="0" x2="0" y2="1">
              <stop offset="0%" stopColor="var(--color-value)" stopOpacity={0.18} />
              <stop offset="100%" stopColor="var(--color-value)" stopOpacity={0.02} />
            </linearGradient>
          </defs>

          {/* Horizontal only, solid hairline: a dashed grid reads as a threshold line. */}
          <CartesianGrid vertical={false} strokeDasharray="" />

          <XAxis
            dataKey="label"
            tickLine={false}
            axisLine={false}
            tickMargin={8}
            minTickGap={24}
          />
          <YAxis
            tickLine={false}
            axisLine={false}
            width={56}
            tickMargin={4}
            tickFormatter={(value: number) => formatMetricCompact(value, metric)}
          />

          {/* The crosshair finds the X so the reader aims at a date, never at a 2px line. */}
          <ChartTooltip
            cursor={{ stroke: "var(--border)", strokeWidth: 1 }}
            content={
              // No `indicator` here: a custom `formatter` replaces the whole item row, the
              // indicator included, so the prop never renders. What it does do is set
              // `nestLabel` on a single-series chart, which moves the bucket label into that
              // discarded row - leaving the tooltip showing a value with no date attached.
              <ChartTooltipContent
                formatter={(value) => formatMetric(Number(value), metric)}
              />
            }
          />

          <Area
            dataKey="value"
            name={metricLabel(metric)}
            type="monotone"
            stroke="var(--color-value)"
            strokeWidth={2}
            fill="url(#issuancesOverTimeFill)"
            // Dots would be a number on every point at day granularity; the crosshair and
            // the table view carry the per-bucket values instead.
            dot={false}
            activeDot={{ r: 4, strokeWidth: 2, stroke: "var(--card)" }}
          />
        </AreaChart>
      </ChartContainer>
    </ChartCard>
  )
}

const GRANULARITY_NOUN: Record<string, string> = {
  hour: "hora",
  day: "dia",
  week: "semana",
  month: "mês",
}

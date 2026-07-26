"use client"

import { useMemo } from "react"
import {
  Bar,
  BarChart,
  Cell,
  LabelList,
  XAxis,
  YAxis,
} from "recharts"
import {
  ChartContainer,
  ChartTooltip,
  ChartTooltipContent,
  type ChartConfig,
} from "@/components/ui/chart"
import type { DashboardBreakdown } from "@/schemas/dashboard"
import type { DashboardMetric } from "@/app/dashboard/dashboard-params"
import { ChartCard } from "./chart-card"
import {
  MAX_CHART_SLICES,
  chartSlot,
  formatMetric,
  formatMetricCompact,
  metricLabel,
  metricValue,
} from "./metric"

interface PointsByProgramChartProps {
  byProgram: DashboardBreakdown[]
  metric: DashboardMetric
}

const BAR_HEIGHT = 34

// Ranked horizontal bars rather than a donut: programme names are long, and a donut only
// answers "share", while the bar answers both "share" and "how much" and stays readable
// past four slices.
//
// Each bar takes a fixed palette slot, so the colour follows the programme and not its
// rank - filtering one out never repaints the survivors. The programme's own registered
// hex still identifies it in the chip beside the name, as it does on the sales screens.
export function PointsByProgramChart({ byProgram, metric }: PointsByProgramChartProps) {
  const data = useMemo(() => foldTail(byProgram, metric), [byProgram, metric])

  const config = {
    value: { label: metricLabel(metric) },
  } satisfies ChartConfig

  const rows = data.map((slice) => ({
    key: slice.id,
    cells: [
      slice.name,
      String(slice.issuanceCount),
      formatMetric(slice.value, metric),
      `${slice.share.toLocaleString("pt-BR", { maximumFractionDigits: 1 })}%`,
    ],
  }))

  return (
    <ChartCard
      title="Por programa de fidelidade"
      description={metricLabel(metric)}
      tableHeaders={["Programa", "Emissões", metricLabel(metric), "Participação"]}
      tableRows={rows}
      isEmpty={data.length === 0}
    >
      <ChartContainer
        config={config}
        className="aspect-auto w-full"
        style={{ height: Math.max(data.length * BAR_HEIGHT + 24, 120) }}
      >
        <BarChart
          data={data}
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
            radius={[0, 4, 4, 0]}
            maxBarSize={24}
          >
            {data.map((slice, index) => (
              <Cell key={slice.id} fill={chartSlot(index)} />
            ))}
            {/* The direct label is what makes the lighter slots legible without leaning on
                fill contrast, and it keeps every value readable without hovering. */}
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

interface ProgramSlice {
  id: string
  name: string
  value: number
  issuanceCount: number
  share: number
}

// Past the eighth slot a ninth hue would be indistinguishable from one already on screen,
// so the tail is folded into a single "Outros" row instead. Zero-value slices are dropped:
// a programme with no issuances in the period is not a bar of length nothing, it is absent.
function foldTail(
  byProgram: DashboardBreakdown[],
  metric: DashboardMetric,
): ProgramSlice[] {
  const slices = byProgram
    .map((program) => ({
      id: program.id,
      name: program.name,
      value: metricValue(program, metric),
      issuanceCount: program.issuance_count,
    }))
    .filter((slice) => slice.value > 0)
    .sort((a, b) => b.value - a.value)

  const total = slices.reduce((sum, slice) => sum + slice.value, 0)
  const withShare = (slice: Omit<ProgramSlice, "share">): ProgramSlice => ({
    ...slice,
    share: total === 0 ? 0 : (slice.value / total) * 100,
  })

  if (slices.length <= MAX_CHART_SLICES) return slices.map(withShare)

  const head = slices.slice(0, MAX_CHART_SLICES - 1)
  const tail = slices.slice(MAX_CHART_SLICES - 1)

  return [
    ...head.map(withShare),
    withShare({
      id: "__outros__",
      name: `Outros (${tail.length})`,
      value: tail.reduce((sum, slice) => sum + slice.value, 0),
      issuanceCount: tail.reduce((sum, slice) => sum + slice.issuanceCount, 0),
    }),
  ]
}

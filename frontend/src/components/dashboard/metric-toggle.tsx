"use client"

import { usePathname, useRouter, useSearchParams } from "next/navigation"
import { Button } from "@/components/ui/button"
import { Label } from "@/components/ui/label"
import { cn } from "@/lib/utils"
import {
  DASHBOARD_METRICS,
  type DashboardMetric,
} from "@/app/dashboard/dashboard-params"

interface MetricToggleProps {
  metric: DashboardMetric
  isAdmin: boolean
}

const METRIC_LABELS: Record<DashboardMetric, string> = {
  points: "Pontos",
  amount: "Reais",
}

// Renders nothing for an operator, the way SalesViewToggle hides the suppliers view. This
// is presentation only - the backend omits amount_cents from the payload for non-admins,
// so hiding the control is a courtesy and not the control that matters.
export function MetricToggle({ metric, isAdmin }: MetricToggleProps) {
  const router = useRouter()
  const pathname = usePathname()
  const searchParams = useSearchParams()

  if (!isAdmin) return null

  function select(next: DashboardMetric) {
    // Merged into the existing URL so switching the unit keeps the period and every
    // dimension the filter bar has already set.
    const params = new URLSearchParams(searchParams.toString())
    params.set("metric", next)
    router.replace(`${pathname}?${params.toString()}`)
  }

  return (
    <div className="flex flex-col gap-1">
      <Label className="text-xs text-slate-500">Medida</Label>
      <div className="flex gap-1">
        {DASHBOARD_METRICS.map((option) => (
          <Button
            key={option}
            type="button"
            size="sm"
            variant="outline"
            onClick={() => select(option)}
            className={cn(
              "font-normal",
              metric === option
                ? "bg-slate-800 text-slate-50 hover:bg-slate-900 hover:text-slate-50"
                : "bg-slate-100 text-slate-600 hover:bg-slate-200",
            )}
          >
            {METRIC_LABELS[option]}
          </Button>
        ))}
      </div>
    </div>
  )
}

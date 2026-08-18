import { AlertTriangle } from "lucide-react"
import { cn } from "@/lib/utils"
import type { CPFUsage } from "@/schemas/account"
import {
  cpfUsageExplanation,
  cpfUsageNoun,
  cpfUsageSeverity,
} from "@/lib/account-category"

const SEVERITY_STYLES = {
  ok: "text-slate-500",
  close: "text-amber-600",
  over: "text-red-600 font-medium",
} as const

// How much of the program's beneficiary cap this account has left. Going over it is not a
// polite error at the airline - the account is blocked, and by then the money is in - so
// the meter is on the card itself rather than behind a click.
export function CPFUsageMeter({ usage }: { usage: CPFUsage }) {
  const severity = cpfUsageSeverity(usage)
  const explanation = cpfUsageExplanation(usage)

  return (
    <span
      className={cn("flex items-center gap-1 text-xs", SEVERITY_STYLES[severity])}
      title={explanation}
    >
      {severity === "over" && <AlertTriangle aria-hidden className="h-3.5 w-3.5" />}
      {usage.used} / {usage.limit} {cpfUsageNoun(usage.limit)}
      {severity === "over" && " — limite atingido"}
    </span>
  )
}

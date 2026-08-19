import { Badge } from "@/components/ui/badge"

/**
 * Marks an account that pays no redemption fee - on Azul, Diamante Unique.
 *
 * Rendered only when the exemption is on: it is the exception, and a badge on every
 * ordinary account saying "pays the fee" would be noise on almost every card. The label
 * names the tier rather than the rule because that is what the operator picks the account
 * by; what the exemption *does* is spelled out on the account form.
 */
export function RedemptionFeeWaiverBadge() {
  return (
    <Badge variant="outline" className="border-sky-300 bg-sky-50 font-normal text-sky-700">
      Isenta de taxa
    </Badge>
  )
}

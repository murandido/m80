import { Badge } from "@/components/ui/badge"
import { LoyaltyProgramSummary } from "@/schemas/account"

interface LoyaltyProgramBadgeProps {
  loyaltyProgram: LoyaltyProgramSummary
}

export function LoyaltyProgramBadge({ loyaltyProgram }: LoyaltyProgramBadgeProps) {
  return (
    <Badge
      style={{ backgroundColor: loyaltyProgram.color, color: "#fff" }}
      className="w-fit"
    >
      {loyaltyProgram.name}
    </Badge>
  )
}

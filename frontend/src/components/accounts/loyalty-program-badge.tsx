import { Badge } from "@/components/ui/badge"
import { LoyaltyProgramSummary } from "@/schemas/account"

interface LoyaltyProgramBadgeProps {
  loyaltyProgram: LoyaltyProgramSummary
  // The issuances grid speaks the same shorthand as the WhatsApp bot, where a
  // program is its code ("SMILES"), never its full name. Everywhere else the
  // name reads better, so that stays the default.
  label?: "name" | "code"
}

export function LoyaltyProgramBadge({
  loyaltyProgram,
  label = "name",
}: LoyaltyProgramBadgeProps) {
  return (
    <Badge
      style={{ backgroundColor: loyaltyProgram.color, color: "#fff" }}
      className="w-fit"
    >
      {label === "code" ? loyaltyProgram.code : loyaltyProgram.name}
    </Badge>
  )
}

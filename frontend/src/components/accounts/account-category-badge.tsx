import { Badge } from "@/components/ui/badge"
import { cn } from "@/lib/utils"
import { accountCategoryLabel } from "@/lib/account-category"

const CATEGORY_STYLES: Record<string, string> = {
  // The one that changes what the operator may do: an injunction suspends the program's
  // beneficiary cap, so this account can keep issuing where the others cannot.
  injunction: "border-violet-300 bg-violet-50 text-violet-700",
  standard: "border-slate-300 text-slate-600",
  // Deliberately quiet. It is the default for everything nobody has classified yet, so it
  // is on most cards on day one and must not shout on all of them.
  unidentified: "border-dashed border-slate-300 text-muted-foreground",
}

export function AccountCategoryBadge({ category }: { category: string }) {
  return (
    <Badge
      variant="outline"
      className={cn("font-normal", CATEGORY_STYLES[category] ?? CATEGORY_STYLES.unidentified)}
    >
      {accountCategoryLabel(category)}
    </Badge>
  )
}

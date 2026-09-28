"use client"

import { useUrlQuery } from "@/hooks/use-url-query"
import { Building2, UserRound } from "lucide-react"
import { Button } from "@/components/ui/button"
import { GROUP_BY_ADMINISTRATOR } from "@/app/dashboard/sales/period-params"
import { cn } from "@/lib/utils"

// Which question the table answers: an account is where the miles came out of, an
// administrator is who gets paid. It writes the URL rather than holding state, so the
// sticky-filter cookie and the PDF link follow along without knowing about it.
export function SupplierGroupToggle({ groupBy }: { groupBy: string }) {
  const { params: urlParams, isPending, update } = useUrlQuery()
  const shownGroupBy = isPending ? (urlParams.get("group_by") ?? "account") : groupBy
  const byAdministrator = shownGroupBy === GROUP_BY_ADMINISTRATOR

  function apply(next: string) {
    update((params) => {
      if (next === GROUP_BY_ADMINISTRATOR) {
        params.set("group_by", GROUP_BY_ADMINISTRATOR)
      } else {
        // Account is the default on both sides, so it is expressed by the key being absent
        // rather than by writing it out.
        params.delete("group_by")
      }
    })
  }

  return (
    <div className="inline-flex rounded-md border bg-white p-0.5" role="group">
      <Button
        type="button"
        size="sm"
        variant="ghost"
        aria-pressed={!byAdministrator}
        className={cn("h-7 px-2.5 text-xs", !byAdministrator && "bg-slate-100 font-medium")}
        onClick={() => apply("account")}
      >
        <Building2 className="h-3.5 w-3.5" />
        Por conta
      </Button>
      <Button
        type="button"
        size="sm"
        variant="ghost"
        aria-pressed={byAdministrator}
        className={cn("h-7 px-2.5 text-xs", byAdministrator && "bg-slate-100 font-medium")}
        onClick={() => apply(GROUP_BY_ADMINISTRATOR)}
      >
        <UserRound className="h-3.5 w-3.5" />
        Por administrador
      </Button>
    </div>
  )
}

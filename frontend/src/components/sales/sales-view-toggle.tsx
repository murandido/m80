import Link from "next/link"
import { Users, Landmark } from "lucide-react"
import { Button } from "@/components/ui/button"
import { cn } from "@/lib/utils"

interface SalesViewToggleProps {
  active: "clients" | "suppliers"
  // What the agency owes each account administrator is not an operator's business, so the
  // whole suppliers view is admin-only. The backend enforces the same on its route.
  canSeeSuppliers: boolean
}

export function SalesViewToggle({ active, canSeeSuppliers }: SalesViewToggleProps) {
  if (!canSeeSuppliers) return null

  return (
    <div className="flex gap-2">
      <Button
        asChild
        type="button"
        className={cn(
          "flex-1 transition-colors duration-200",
          active === "clients"
            ? "bg-slate-800 hover:bg-slate-900"
            : "bg-slate-100 text-slate-500 hover:bg-slate-200",
        )}
      >
        <Link href="/dashboard/sales">
          <div
            className={cn(
              "flex w-full items-center justify-center gap-2 p-1 text-sm",
              active === "clients" && "text-slate-50",
            )}
          >
            <Users className="h-4 w-4" />
            Clientes
          </div>
        </Link>
      </Button>
      <Button
        asChild
        type="button"
        className={cn(
          "flex-1 transition-colors duration-200",
          active === "suppliers"
            ? "bg-slate-800 hover:bg-slate-900"
            : "bg-slate-100 text-slate-500 hover:bg-slate-200",
        )}
      >
        <Link href="/dashboard/sales/suppliers">
          <div
            className={cn(
              "flex w-full items-center justify-center gap-2 p-1 text-sm",
              active === "suppliers" && "text-slate-50",
            )}
          >
            <Landmark className="h-4 w-4" />
            Fornecedores
          </div>
        </Link>
      </Button>
    </div>
  )
}

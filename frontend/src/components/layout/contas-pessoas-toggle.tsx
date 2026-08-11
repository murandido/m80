import Link from "next/link"
import { Landmark, Users } from "lucide-react"
import { Button } from "@/components/ui/button"
import { cn } from "@/lib/utils"

interface ContasPessoasToggleProps {
  active: "contas" | "pessoas"
}

export function ContasPessoasToggle({ active }: ContasPessoasToggleProps) {
  return (
    <div className="flex gap-2">
      <Button
        asChild
        type="button"
        className={cn(
          "flex-1 transition-colors duration-200",
          active === "contas"
            ? "bg-slate-800 hover:bg-slate-900"
            : "bg-slate-100 text-slate-500 hover:bg-slate-200",
        )}
      >
        <Link href="/dashboard/accounts">
          <div
            className={cn(
              "flex w-full items-center justify-center gap-2 p-1 text-sm",
              active === "contas" && "text-slate-50",
            )}
          >
            <Landmark className="h-4 w-4" />
            Contas
          </div>
        </Link>
      </Button>
      <Button
        asChild
        type="button"
        className={cn(
          "flex-1 transition-colors duration-200",
          active === "pessoas"
            ? "bg-slate-800 hover:bg-slate-900"
            : "bg-slate-100 text-slate-500 hover:bg-slate-200",
        )}
      >
        <Link href="/dashboard/people">
          <div
            className={cn(
              "flex w-full items-center justify-center gap-2 p-1 text-sm",
              active === "pessoas" && "text-slate-50",
            )}
          >
            <Users className="h-4 w-4" />
            Proprietários
          </div>
        </Link>
      </Button>
    </div>
  )
}

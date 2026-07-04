"use client"

import Link from "next/link"
import { Award, Plane } from "lucide-react"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { cn } from "@/lib/utils"

interface LoyaltyProgramsAirlinesToggleProps {
  active: "loyalty-programs" | "airlines"
}

export function LoyaltyProgramsAirlinesToggle({
  active,
}: LoyaltyProgramsAirlinesToggleProps) {
  return (
    <div className="flex gap-2">
      <Button
        type="button"
        disabled
        className="flex-1 cursor-not-allowed bg-slate-100 text-slate-400 hover:bg-slate-100"
      >
        <div className="flex w-full items-center justify-center gap-2 p-1 text-sm">
          <Award className="h-4 w-4" />
          Programas de Fidelidade
          <Badge variant="outline" className="text-[10px] text-slate-400">
            Em breve
          </Badge>
        </div>
      </Button>
      <Button
        asChild
        type="button"
        className={cn(
          "flex-1 transition-colors duration-200",
          active === "airlines"
            ? "bg-slate-800 hover:bg-slate-900"
            : "bg-slate-100 text-slate-500 hover:bg-slate-200",
        )}
      >
        <Link href="/dashboard/airlines">
          <div
            className={cn(
              "flex w-full items-center justify-center gap-2 p-1 text-sm",
              active === "airlines" && "text-slate-50",
            )}
          >
            <Plane className="h-4 w-4" />
            CIAs Aéreas
          </div>
        </Link>
      </Button>
    </div>
  )
}

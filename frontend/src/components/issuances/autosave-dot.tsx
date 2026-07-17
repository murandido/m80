import { cn } from "@/lib/utils"
import { SaveStatus } from "@/components/issuances/types"

export function AutosaveDot({ status }: { status: SaveStatus }) {
  if (status === "idle" || status === "saving") return null

  return (
    <span
      className={cn(
        "block h-2 w-2 rounded-full",
        status === "success" && "bg-emerald-500",
        status === "error" && "bg-red-500",
      )}
    />
  )
}

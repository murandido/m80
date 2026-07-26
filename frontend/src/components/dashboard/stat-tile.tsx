import { ArrowDown, ArrowUp, Minus } from "lucide-react"
import { Card, CardContent } from "@/components/ui/card"
import { cn } from "@/lib/utils"

interface StatTileProps {
  label: string
  value: string
  // The same figure for the previous period, already formatted, shown under the delta so
  // the percentage is never the only thing to go on.
  previousValue?: string
  // Raw numbers for the delta. Undefined on either side means there is nothing to compare
  // against, and the tile simply omits the row.
  current?: number
  previous?: number
  // Whether a rise is good news. Cancellations rise the wrong way, so the arrow's colour
  // cannot be derived from the sign alone.
  upIsGood?: boolean
}

// A single figure is a stat tile, not a one-bar chart. The value uses the font's default
// proportional figures: `tabular-nums` gives every digit the width of a zero, which makes a
// number like 121 look loose at this size. Columns of numbers (the table views) still get
// tabular figures, where vertical alignment is the point.
export function StatTile({
  label,
  value,
  previousValue,
  current,
  previous,
  upIsGood = true,
}: StatTileProps) {
  const delta = resolveDelta(current, previous)

  return (
    <Card className="gap-0 py-4 shadow-none">
      <CardContent className="px-4">
        <p className="text-xs font-medium text-slate-500">{label}</p>
        <p className="mt-1 text-2xl font-semibold text-slate-800">{value}</p>

        {delta === null ? null : (
          <div className="mt-1 flex items-center gap-1 text-xs">
            <DeltaIcon direction={delta.direction} upIsGood={upIsGood} />
            <span
              className={cn(
                "font-medium",
                deltaToneClass(delta.direction, upIsGood),
              )}
            >
              {delta.label}
            </span>
            {previousValue ? (
              <span className="text-slate-400">vs. {previousValue}</span>
            ) : null}
          </div>
        )}
      </CardContent>
    </Card>
  )
}

type DeltaDirection = "up" | "down" | "flat"

function resolveDelta(
  current: number | undefined,
  previous: number | undefined,
): { direction: DeltaDirection; label: string } | null {
  if (current === undefined || previous === undefined) return null

  if (previous === 0) {
    // A percentage against zero is either infinity or a lie. "Sem base" says what actually
    // happened - there was nothing in the previous period to compare against.
    if (current === 0) return { direction: "flat", label: "Sem variação" }
    return { direction: "up", label: "Sem base anterior" }
  }

  const change = ((current - previous) / previous) * 100
  if (Math.abs(change) < 0.05) return { direction: "flat", label: "Sem variação" }

  return {
    direction: change > 0 ? "up" : "down",
    label: `${change > 0 ? "+" : ""}${change.toLocaleString("pt-BR", {
      maximumFractionDigits: 1,
    })}%`,
  }
}

// Direction is carried by an icon as well as by colour, so the tile still reads correctly
// in greyscale or with a colour vision deficiency.
function DeltaIcon({
  direction,
  upIsGood,
}: {
  direction: DeltaDirection
  upIsGood: boolean
}) {
  const className = cn("h-3 w-3 shrink-0", deltaToneClass(direction, upIsGood))
  if (direction === "up") return <ArrowUp className={className} />
  if (direction === "down") return <ArrowDown className={className} />
  return <Minus className={className} />
}

function deltaToneClass(direction: DeltaDirection, upIsGood: boolean): string {
  if (direction === "flat") return "text-slate-400"
  const isGood = direction === "up" ? upIsGood : !upIsGood
  return isGood ? "text-emerald-700" : "text-red-600"
}

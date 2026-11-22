import { ReactNode } from "react"
import { cn } from "@/lib/utils"
import { AccountMatch } from "@/lib/account-match"

// Past this many the row would wrap into a paragraph; the rest are counted instead.
const MAX_VISIBLE_ALIASES = 6

interface AccountMatchOptionProps {
  match: AccountMatch
  // Extra line under the aliases - the program, where the list mixes programs.
  footer?: ReactNode
}

// One account in a picker list: its main name large, and under it, smaller, the aliases
// that answer what was typed, closest first. The closest one is highlighted so the operator
// sees that what they typed is an alias of this account - the main name above is what the
// issuance will show.
export function AccountMatchOption({ match, footer }: AccountMatchOptionProps) {
  const visible = match.aliases.slice(0, MAX_VISIBLE_ALIASES)
  const hidden = match.aliases.length - visible.length

  return (
    <span className="flex min-w-0 flex-1 flex-col gap-0.5">
      <span className="truncate text-sm font-medium">{match.mainName}</span>
      {visible.length > 0 && (
        <span className="text-muted-foreground flex flex-wrap items-center gap-x-1.5 gap-y-1 text-xs">
          {visible.map((alias) => (
            <span
              key={alias.id}
              // Every alias is its own chip so neighbours read apart ("Ana - R" next to
              // "Ana Moura" used to run together); the closest one is the orange chip. The grey
              // one carries a ring because the filter bars paint the active row slate-200 too.
              className={cn(
                "max-w-full truncate rounded px-1",
                alias.id === match.highlightedAliasId
                  ? "bg-orange-200 text-orange-950"
                  : "bg-slate-100 text-slate-700 ring-1 ring-slate-300 ring-inset",
              )}
            >
              {alias.nickname}
            </span>
          ))}
          {hidden > 0 && <span>+{hidden}</span>}
        </span>
      )}
      {footer}
    </span>
  )
}

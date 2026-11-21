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
        <span className="text-muted-foreground flex flex-wrap items-center gap-x-2 gap-y-0.5 text-xs">
          {visible.map((alias) => (
            <span
              key={alias.id}
              className={cn(
                "max-w-full truncate",
                alias.id === match.highlightedAliasId &&
                  "rounded bg-orange-200 px-1 text-orange-950",
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

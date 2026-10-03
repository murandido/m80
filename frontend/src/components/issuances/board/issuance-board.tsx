"use client"

import { memo, useRef } from "react"
import Color from "color"
import { Building2, Hash, Loader2, Plane, Plus, UserRound } from "lucide-react"
import { cn } from "@/lib/utils"
import { accountFallbackLabel } from "@/components/issuances/emissao-message"
import type { IssuanceRow } from "@/components/issuances/types"
import type { AccountOption } from "@/schemas/account"
import type { Agency } from "@/schemas/agency"
import type { LoyaltyProgram } from "@/schemas/loyalty-program"
import type { PersonOption } from "@/schemas/person"

// How a card is coloured: the loyalty program's own colour, with the text in whichever of
// white or near-black reads on it. Cancelled and unsaved cards opt out - grey for the
// first (red is taken by programs, so it cannot mean "cancelled" here), white for the second.
export function cardPalette(
  row: IssuanceRow,
  program: LoyaltyProgram | undefined,
) {
  if (row.status === "cancelled") {
    return { background: "#cbd5e1", foreground: "#334155", tinted: true }
  }
  if (row.isDraft || !program) {
    return { background: "#ffffff", foreground: "#1e293b", tinted: false }
  }
  let light = false
  try {
    // Higher than Color's own isLight cut-off, which reads brand orange as light; the
    // design writes white on every program colour but the palest.
    light = Color(program.color).luminosity() > 0.55
  } catch {
    // A colour the parser rejects still gets a readable card.
  }
  return {
    background: program.color,
    foreground: light ? "#1e293b" : "#ffffff",
    tinted: true,
  }
}

// The alias the row points at, or how an account with no alias is named everywhere else.
export function accountLabelFor(
  row: IssuanceRow,
  accounts: AccountOption[],
  people: PersonOption[],
): string | null {
  const account = accounts.find((candidate) => candidate.id === row.account_id)
  if (!account) return null
  const nickname = account.nicknames.find(
    (entry) => entry.id === row.account_nickname_id,
  )
  return nickname?.nickname ?? accountFallbackLabel(account, people)
}

interface IssuanceBoardProps {
  rows: IssuanceRow[]
  loyaltyPrograms: LoyaltyProgram[]
  accounts: AccountOption[]
  agencies: Agency[]
  people: PersonOption[]
  openKey: string | null
  busyKeys: Record<string, true>
  pendingAccountLabels: Record<string, string>
  onOpen: (key: string) => void
  onAdd: () => void
}

export function IssuanceBoard({
  rows,
  loyaltyPrograms,
  accounts,
  agencies,
  people,
  openKey,
  busyKeys,
  pendingAccountLabels,
  onOpen,
  onAdd,
}: IssuanceBoardProps) {
  const gridRef = useRef<HTMLDivElement>(null)

  // Arrow keys walk the cards the way they sit on screen. Up and down look for the card in
  // the row above or below whose left edge is closest, since how many cards fit in a row
  // depends on the width.
  function handleKeyDown(event: React.KeyboardEvent<HTMLDivElement>) {
    if (
      !["ArrowLeft", "ArrowRight", "ArrowUp", "ArrowDown"].includes(event.key)
    )
      return
    const cards = Array.from(
      gridRef.current?.querySelectorAll<HTMLElement>("[data-card]") ?? [],
    )
    const current = cards.indexOf(document.activeElement as HTMLElement)
    if (current === -1) return
    event.preventDefault()

    if (event.key === "ArrowLeft")
      return cards[Math.max(current - 1, 0)]?.focus()
    if (event.key === "ArrowRight")
      return cards[Math.min(current + 1, cards.length - 1)]?.focus()

    const here = cards[current].getBoundingClientRect()
    const below = event.key === "ArrowDown"
    const candidates = cards.filter((card) => {
      const rect = card.getBoundingClientRect()
      return below ? rect.top > here.top + 4 : rect.top < here.top - 4
    })
    if (candidates.length === 0) return
    const rowTop = below
      ? Math.min(...candidates.map((card) => card.getBoundingClientRect().top))
      : Math.max(...candidates.map((card) => card.getBoundingClientRect().top))
    const sameRow = candidates.filter(
      (card) => Math.abs(card.getBoundingClientRect().top - rowTop) < 4,
    )
    sameRow
      .sort(
        (a, b) =>
          Math.abs(a.getBoundingClientRect().left - here.left) -
          Math.abs(b.getBoundingClientRect().left - here.left),
      )[0]
      ?.focus()
  }

  return (
    <div
      ref={gridRef}
      onKeyDown={handleKeyDown}
      className="grid grid-cols-1 gap-3 sm:grid-cols-[repeat(auto-fill,minmax(9.5rem,1fr))] sm:gap-4"
    >
      {rows.map((row, index) => (
        <IssuanceCard
          key={row.key}
          row={row}
          number={index + 1}
          program={loyaltyPrograms.find(
            (program) => program.id === row.loyalty_program_id,
          )}
          agencyName={
            agencies.find((agency) => agency.id === row.agency_id)?.name ?? null
          }
          accountLabel={
            pendingAccountLabels[row.key] ??
            accountLabelFor(row, accounts, people)
          }
          isOpen={row.key === openKey}
          isBusy={Boolean(busyKeys[row.key]) || Boolean(row.traveos_syncing_at)}
          onOpen={onOpen}
        />
      ))}
      {/* Padded like the cards' selection frame, so the "+" lines up with them. */}
      <div className="h-full p-1">
        <button
          type="button"
          data-card
          onClick={onAdd}
          title="Nova emissão (Ctrl+Enter)"
          aria-label="Nova emissão"
          className="focus-visible:ring-ring/50 flex min-h-24 w-full items-center justify-center rounded-lg border border-slate-200 bg-white text-slate-300 transition-colors outline-none hover:bg-slate-50 hover:text-slate-400 focus-visible:ring-[3px] sm:h-full sm:min-h-36"
        >
          <Plus className="h-14 w-14" strokeWidth={2.5} />
        </button>
      </div>
    </div>
  )
}

interface IssuanceCardProps {
  row: IssuanceRow
  number: number
  program: LoyaltyProgram | undefined
  agencyName: string | null
  accountLabel: string | null
  isOpen: boolean
  isBusy: boolean
  onOpen: (key: string) => void
}

const IssuanceCard = memo(function IssuanceCard({
  row,
  number,
  program,
  agencyName,
  accountLabel,
  isOpen,
  isBusy,
  onOpen,
}: IssuanceCardProps) {
  const palette = cardPalette(row, program)
  const locator = row.airline_locator?.trim() || row.program_locator.trim()
  const status =
    row.status === "cancelled"
      ? "Cancelado"
      : row.status === "complete"
        ? "Completo"
        : "Incompleto"

  return (
    // The frame is what marks the open card, the way the design draws it: a pale box
    // around the card rather than a change to the card itself.
    <div
      className={cn(
        "h-full rounded-xl p-1 transition-colors",
        isOpen ? "bg-slate-200" : "bg-transparent",
      )}
    >
      <button
        type="button"
        data-card
        data-card-key={row.key}
        onClick={() => onOpen(row.key)}
        aria-label={`Emissão ${number}, ${status}`}
        aria-pressed={isOpen}
        style={{
          backgroundColor: palette.background,
          color: palette.foreground,
        }}
        className={cn(
          "focus-visible:ring-ring/60 flex w-full flex-col gap-2 rounded-lg p-2.5 text-left text-sm outline-none focus-visible:ring-[3px] sm:h-full sm:text-xs",
          !palette.tinted && "border border-slate-200",
          row.hasError && "ring-2 ring-red-600",
        )}
      >
        <div
          className="flex items-center justify-between border-b pb-2"
          style={{
            borderColor: palette.tinted ? "rgb(255 255 255 / 0.7)" : "#e2e8f0",
          }}
        >
          <span className="flex h-6 min-w-6 items-center justify-center rounded-full bg-white px-1.5 text-xs font-medium text-slate-900 tabular-nums">
            {number}
          </span>
          <StatusDot row={row} isBusy={isBusy} label={status} />
        </div>
        <CardLine
          icon={Plane}
          tinted={palette.tinted}
          value={program?.name ?? null}
        />
        <CardLine
          icon={Hash}
          tinted={palette.tinted}
          value={locator || null}
          className={row.status === "cancelled" ? "line-through" : undefined}
        />
        <CardLine icon={Building2} tinted={palette.tinted} value={agencyName} />
        <CardLine
          icon={UserRound}
          tinted={palette.tinted}
          value={accountLabel}
        />
      </button>
    </div>
  )
})

function StatusDot({
  row,
  isBusy,
  label,
}: {
  row: IssuanceRow
  isBusy: boolean
  label: string
}) {
  if (isBusy || row.saveStatus === "saving") {
    return (
      <span
        title="Salvando ou puxando dados"
        className="flex h-6 w-6 items-center justify-center rounded-full bg-white"
      >
        <Loader2 className="h-3.5 w-3.5 animate-spin text-sky-600" />
      </span>
    )
  }
  return (
    <span
      title={label}
      className="flex h-6 w-6 items-center justify-center rounded-full bg-white/80"
    >
      <span
        className={cn(
          "h-3.5 w-3.5 rounded-full",
          row.status === "complete" && "bg-emerald-600",
          row.status === "incomplete" && "bg-red-600",
          row.status === "cancelled" && "bg-slate-500",
        )}
      />
    </span>
  )
}

function CardLine({
  icon: Icon,
  value,
  tinted,
  className,
}: {
  icon: typeof Plane
  value: string | null
  tinted: boolean
  className?: string
}) {
  return (
    <span className="flex min-w-0 items-center gap-1.5">
      <span
        className={cn(
          "flex h-6 w-6 shrink-0 items-center justify-center rounded-full sm:h-5 sm:w-5",
          tinted ? "bg-white text-slate-900" : "bg-slate-100 text-slate-600",
        )}
      >
        <Icon className="h-3.5 w-3.5 sm:h-3 sm:w-3" />
      </span>
      <span className={cn("truncate font-medium", className)}>
        {value ?? "…"}
      </span>
    </span>
  )
}

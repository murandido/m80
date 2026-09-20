"use client"

import { memo, useEffect, useRef, useState } from "react"
import { cn } from "@/lib/utils"
import {
  HoverCard,
  HoverCardContent,
  HoverCardTrigger,
} from "@/components/ui/hover-card"
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip"
import { CellStateProps, useGridContext } from "@/components/issuances/grid-context"
import { IssuanceRow } from "@/components/issuances/types"
import { IssuanceStatus } from "@/schemas/issuance"
import { formatCurrencyBRL } from "@/lib/currency"
import { User } from "@/schemas/user"
import { Direction } from "@/lib/grid-navigation"
import { DropdownPortal } from "@/components/issuances/cells/dropdown-portal"

// The pill's colour by status, shared by the read view and the edit-mode anchor so the
// highlighted option looks the same in both. "cancelled" can only appear in the read view.
function statusPillClass(status: IssuanceStatus): string {
  return cn(
    "rounded-full px-2 py-0.5 text-xs font-medium",
    status === "incomplete" && "bg-amber-100 text-amber-800",
    status === "complete" && "bg-emerald-100 text-emerald-800",
    status === "cancelled" && "bg-red-100 text-red-800",
  )
}

// The read view is just a dot: the column sits last and narrow, and the label is one
// hover away. A solid core inside a pale halo of the same hue, the look of the reference
// the designer drew; same colour family as the pill the edit view still uses.
function statusDotHaloClass(status: IssuanceStatus): string {
  return cn(
    "flex h-4 w-4 items-center justify-center rounded-full",
    status === "incomplete" && "bg-amber-100",
    status === "complete" && "bg-emerald-100",
    status === "cancelled" && "bg-red-100",
  )
}

function statusDotCoreClass(status: IssuanceStatus): string {
  return cn(
    "h-2 w-2 rounded-full",
    status === "incomplete" && "bg-amber-500",
    status === "complete" && "bg-emerald-700",
    status === "cancelled" && "bg-red-600",
  )
}

const STATUS_LABELS: Record<IssuanceStatus, string> = {
  incomplete: "Incompleto",
  complete: "Completo",
  cancelled: "Cancelado",
}

// "cancelled" is intentionally absent: cancelling (and reverting) go through the dedicated
// row buttons, which capture the refund amounts / restore sale state - picking it here would
// bypass all of that. It stays in STATUS_LABELS above for displaying an already-cancelled row.
const STATUS_OPTIONS: IssuanceStatus[] = ["incomplete", "complete"]

// The details captured at cancellation time, shown on hover over a cancelled row's pill so
// the operator can see who reverted the money and how much, without leaving the grid.
function CancellationDetails({ row, users }: { row: IssuanceRow; users: User[] }) {
  const cancellerName = users.find((u) => u.id === row.cancelled_by_id)?.name ?? "—"
  const cancelledAt = row.cancelled_at
    ? new Date(row.cancelled_at).toLocaleString("pt-BR")
    : "—"
  const hasRefund =
    row.refunded_points !== null ||
    row.refunded_taxes !== null ||
    row.cancellation_fee !== null

  return (
    <div className="flex flex-col gap-2 text-xs">
      <div className="grid grid-cols-[auto_1fr] gap-x-3 gap-y-1">
        <span className="text-muted-foreground">Cancelada por</span>
        <span className="font-medium">{cancellerName}</span>
        <span className="text-muted-foreground">Em</span>
        <span className="font-medium">{cancelledAt}</span>
      </div>
      {row.cancellation_only && (
        <div className="border-t pt-2 text-amber-700">
          Estorno avulso: a reserva não existia no painel, então os pontos não contam
          como emitidos — só o reembolso entra no saldo do fornecedor.
        </div>
      )}
      <div className="border-t pt-2">
        {hasRefund ? (
          <div className="grid grid-cols-[1fr_auto] gap-x-3 gap-y-1">
            {row.refunded_points !== null && (
              <>
                <span className="text-muted-foreground">Pontos reembolsados</span>
                <span className="font-medium tabular-nums">{row.refunded_points}</span>
              </>
            )}
            {row.refunded_taxes !== null && (
              <>
                <span className="text-muted-foreground">Taxas reembolsadas</span>
                <span className="font-medium tabular-nums">
                  {formatCurrencyBRL(row.refunded_taxes)}
                </span>
              </>
            )}
            {row.cancellation_fee !== null && (
              <>
                <span className="text-muted-foreground">Multa de cancelamento</span>
                <span className="font-medium tabular-nums">
                  {formatCurrencyBRL(row.cancellation_fee)}
                </span>
              </>
            )}
          </div>
        ) : (
          <span className="text-muted-foreground">Sem reembolso</span>
        )}
      </div>
    </div>
  )
}

export const StatusCell = memo(function StatusCell({
  r,
  c,
  row,
  isEditing,
}: { r: number; c: number } & Pick<CellStateProps, "row" | "isEditing">) {
  const { onCellClick, onSaveAndMove, onExitEdit, registerActiveCellSave, users } =
    useGridContext()
  const isThisCellEditing = isEditing

  // Index of the highlighted option in STATUS_OPTIONS. The dropdown stays open the whole
  // time the cell is editing (that is what makes it drop down on entry, keyboard-first).
  const [activeIndex, setActiveIndex] = useState(0)
  const anchorRef = useRef<HTMLButtonElement>(null)
  const savedRef = useRef(false)

  useEffect(() => {
    if (isThisCellEditing) {
      const idx = STATUS_OPTIONS.indexOf(row.status)
      setActiveIndex(idx === -1 ? 0 : idx)
      savedRef.current = false
      requestAnimationFrame(() => anchorRef.current?.focus())
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [isThisCellEditing])

  function commitIndex(index: number, direction: Direction | null) {
    if (savedRef.current) return
    savedRef.current = true
    onSaveAndMove(r, { status: STATUS_OPTIONS[index] }, direction)
  }

  useEffect(() => {
    if (isThisCellEditing) {
      registerActiveCellSave(() => commitIndex(activeIndex, null))
      return () => registerActiveCellSave(null)
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [isThisCellEditing, activeIndex])

  if (!isThisCellEditing) {
    const dot = (
      <button
        type="button"
        onClick={() => onCellClick(r, c)}
        aria-label={STATUS_LABELS[row.status]}
        className="flex w-full items-center justify-center px-1 py-2"
      >
        <span className={statusDotHaloClass(row.status)}>
          <span className={statusDotCoreClass(row.status)} />
        </span>
        {/* Kept in the DOM for Ctrl+C, which copies the cell's rendered text. */}
        <span className="sr-only">{STATUS_LABELS[row.status]}</span>
      </button>
    )

    // Hovering a cancelled row's dot reveals who cancelled it, when, and the refund
    // breakdown. Click still selects the cell (the trigger wraps the same button).
    if (row.status === "cancelled") {
      return (
        <HoverCard openDelay={150}>
          <HoverCardTrigger asChild>{dot}</HoverCardTrigger>
          <HoverCardContent className="w-64">
            <p className="mb-2 text-xs font-medium">{STATUS_LABELS.cancelled}</p>
            <CancellationDetails row={row} users={users} />
          </HoverCardContent>
        </HoverCard>
      )
    }

    return (
      <Tooltip>
        <TooltipTrigger asChild>{dot}</TooltipTrigger>
        <TooltipContent>{STATUS_LABELS[row.status]}</TooltipContent>
      </Tooltip>
    )
  }

  // Edit mode: a focusable anchor showing the highlighted option as a pill, with an
  // always-open portal list below it - mirroring ComboboxCell so Status behaves like every
  // other dropdown column (opens on entry, arrow to choose, Enter/Tab to commit, no mouse).
  return (
    <div className="relative">
      <button
        ref={anchorRef}
        type="button"
        onKeyDown={(e) => {
          if (e.key === "Escape") {
            e.preventDefault()
            onExitEdit()
          } else if (e.key === "ArrowDown") {
            e.preventDefault()
            setActiveIndex((i) => Math.min(i + 1, STATUS_OPTIONS.length - 1))
          } else if (e.key === "ArrowUp") {
            e.preventDefault()
            setActiveIndex((i) => Math.max(i - 1, 0))
          } else if (e.key === "Enter") {
            e.preventDefault()
            commitIndex(activeIndex, e.shiftKey ? "up" : "down")
          } else if (e.key === "Tab") {
            e.preventDefault()
            commitIndex(activeIndex, e.shiftKey ? "tab-left" : "tab-right")
          }
        }}
        onBlur={() => commitIndex(activeIndex, null)}
        className="block w-full px-2 py-1.5 text-left outline-none ring-2 ring-inset ring-primary"
      >
        <span className={statusPillClass(STATUS_OPTIONS[activeIndex])}>
          {STATUS_LABELS[STATUS_OPTIONS[activeIndex]]}
        </span>
      </button>
      <DropdownPortal anchorRef={anchorRef}>
        {STATUS_OPTIONS.map((status, index) => (
          <button
            key={status}
            type="button"
            // preventDefault keeps focus on the anchor so onBlur doesn't fire (and commit the
            // highlighted option) before this click commits the clicked one.
            onMouseDown={(e) => e.preventDefault()}
            onClick={() => commitIndex(index, null)}
            className={cn(
              "block w-full px-2 py-1.5 text-left text-sm hover:bg-accent",
              index === activeIndex && "bg-accent",
            )}
          >
            {STATUS_LABELS[status]}
          </button>
        ))}
      </DropdownPortal>
    </div>
  )
})

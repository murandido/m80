"use client"

import { useEffect, useMemo, useReducer, useRef, useState } from "react"
import { Plus } from "lucide-react"
import { toast } from "sonner"
import { cn } from "@/lib/utils"
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table"
import { Button } from "@/components/ui/button"
import { GridProvider, GridReferenceData } from "@/components/issuances/grid-context"
import { GridCell } from "@/components/issuances/grid-cell"
import { RowActions } from "@/components/issuances/row-actions"
import { AutosaveDot } from "@/components/issuances/autosave-dot"
import { interactionReducer, initialInteractionState } from "@/components/issuances/interaction-reducer"
import { IssuanceRow, issuanceToRow, makeDraftRow, draftIsReadyToCreate } from "@/components/issuances/types"
import {
  ACCOUNT_COLUMN_INDEX,
  CALCULATED_COLUMN_INDEX,
  COLUMN_COUNT,
  clampMove,
  resolveNextCell,
  Direction,
} from "@/lib/grid-navigation"
import { Issuance, updateIssuanceSchema } from "@/schemas/issuance"
import { Account } from "@/schemas/account"
import { createIssuanceAction, updateIssuanceAction, deleteIssuanceAction } from "@/actions/issuance"
import { createAccountForIssuanceAction } from "@/actions/account"
import { z } from "zod"

const COLUMN_LABELS = [
  "Cia aérea",
  "Localizador",
  "Data do voo",
  "Passageiro principal",
  "Origem",
  "Destino",
  "Pontos",
  "Valor milheiro",
  "Taxas",
  "Valor total",
  "Conta",
  "Emissor",
  "Agência",
  "Status",
]

interface IssuancesGridProps extends GridReferenceData {
  initialIssuances: Issuance[]
  // Defaults a newly created draft row's "Emissor" - still freely editable.
  currentUserId: string | null
}

// A draft row nobody has touched yet - clicking "Nova emissão" repeatedly
// should focus this one instead of piling up empty rows.
function isUntouchedDraft(row: IssuanceRow): boolean {
  return (
    row.isDraft &&
    row.program_locator.trim() === "" &&
    row.loyalty_program_id === null &&
    row.account_id === null &&
    row.points === null &&
    row.value_of_the_thousand === null
  )
}

export function IssuancesGrid({ initialIssuances, currentUserId, ...referenceData }: IssuancesGridProps) {
  const [rows, setRows] = useState<IssuanceRow[]>(() => {
    const mapped = initialIssuances.map(issuanceToRow)
    return mapped.length > 0 ? mapped : [makeDraftRow(currentUserId)]
  })
  const [interaction, dispatch] = useReducer(interactionReducer, initialInteractionState)
  const activeCellSaveRef = useRef<(() => void) | null>(null)
  const containerRef = useRef<HTMLDivElement>(null)
  // Rows whose POST is currently in flight. `row.isDraft` alone can't gate the create:
  // it comes from the current render, so a cell edited before the POST resolves would
  // still see isDraft === true and fire a second create for the same row.
  const creatingKeysRef = useRef(new Set<string>())
  // Edits made while a row's POST is in flight, replayed as a PATCH once the server
  // hands back the real id.
  const queuedPatchesRef = useRef(new Map<string, Partial<IssuanceRow>>())
  // Rows whose account the program change wiped, until a new one is picked. Session-only
  // by nature: the old account_id survives in the database (the column is NOT NULL), so a
  // reload brings it back and the highlight legitimately disappears.
  const [accountClearedKeys, setAccountClearedKeys] = useState(new Set<string>())
  // Accounts created inline from the account cell. Kept alongside the prop rather than
  // replacing it, so a later refresh of the server-fetched list doesn't get shadowed by
  // a stale copy - the merge below dedupes when the same account arrives from both.
  const [createdAccounts, setCreatedAccounts] = useState<Account[]>([])
  const [pendingAccountLabels, setPendingAccountLabels] = useState<Record<string, string>>({})
  // Lets the async account creation read the rows as they are when it resolves, not as
  // they were when it started.
  const rowsRef = useRef(rows)

  const accounts = useMemo(() => {
    const byId = new Map(referenceData.accounts.map((account) => [account.id, account]))
    for (const account of createdAccounts) {
      if (!byId.has(account.id)) byId.set(account.id, account)
    }
    return [...byId.values()]
  }, [referenceData.accounts, createdAccounts])

  useEffect(() => {
    rowsRef.current = rows
  }, [rows])

  useEffect(() => {
    if (!interaction.editing) {
      containerRef.current?.focus()
    }
  }, [interaction.editing, interaction.sel.r, interaction.sel.c])

  function updateRow(key: string, patch: Partial<IssuanceRow>) {
    setRows((prev) => prev.map((row) => (row.key === key ? { ...row, ...patch } : row)))
  }

  function removeRow(key: string) {
    setRows((prev) => {
      const next = prev.filter((row) => row.key !== key)
      return next.length > 0 ? next : [makeDraftRow(currentUserId)]
    })
    setAccountClearedKeys((prev) => {
      if (!prev.has(key)) return prev
      const next = new Set(prev)
      next.delete(key)
      return next
    })
  }

  function flashSaveStatus(key: string, status: "success" | "error") {
    // hasError outlives the flash: the dot resets to idle after 900ms, but a failed row
    // stays flagged red until a later save succeeds and clears it here.
    updateRow(key, { saveStatus: status, hasError: status === "error" })
    setTimeout(() => updateRow(key, { saveStatus: "idle" }), 900)
  }

  // The backend fills defaults the client can't know (airline from the loyalty program,
  // the default agency, issuer from the JWT) and derives `status`, so a created row is
  // re-hydrated from the response instead of being patched with just the new id. The
  // draft's key is kept - issuanceToRow would swap in the backend id, remounting the row
  // and dropping the user's cell selection mid-typing.
  function reconcileCreatedRow(key: string, issuance: Issuance) {
    setRows((prev) =>
      prev.map((row) =>
        row.key === key ? { ...issuanceToRow(issuance), key, saveStatus: row.saveStatus } : row,
      ),
    )
  }

  async function persist(row: IssuanceRow, patch: Partial<IssuanceRow>) {
    const merged = { ...row, ...patch }

    if (row.isDraft) {
      if (creatingKeysRef.current.has(row.key)) {
        queuedPatchesRef.current.set(row.key, { ...queuedPatchesRef.current.get(row.key), ...patch })
        return
      }

      if (!draftIsReadyToCreate(merged)) return

      creatingKeysRef.current.add(row.key)
      updateRow(row.key, { saveStatus: "saving" })
      const result = await createIssuanceAction({
        program_locator: merged.program_locator,
        loyalty_program_id: merged.loyalty_program_id!,
        account_id: merged.account_id!,
        account_nickname_id: merged.account_nickname_id ?? undefined,
        points: merged.points!,
        value_of_the_thousand: merged.value_of_the_thousand!,
        airline_id: merged.airline_id ?? undefined,
        agency_id: merged.agency_id ?? undefined,
        issuer_id: merged.issuer_id ?? undefined,
        // `||` and not `??`: the locator cell writes an empty string to mean "no airline
        // locator", and a create has nothing to clear - the field is simply omitted.
        airline_locator: merged.airline_locator || undefined,
        main_passenger_name: merged.main_passenger_name ?? undefined,
        origin_iata_code: merged.origin_iata_code ?? undefined,
        destination_iata_code: merged.destination_iata_code ?? undefined,
        taxes: merged.taxes ?? undefined,
        flight_date: merged.flight_date ?? undefined,
      }).finally(() => creatingKeysRef.current.delete(row.key))

      const queued = queuedPatchesRef.current.get(row.key)
      queuedPatchesRef.current.delete(row.key)

      if ("error" in result) {
        toast.error(result.error)
        flashSaveStatus(row.key, "error")
        // The row stays a draft holding those edits in local state, so the next cell
        // save retries the create with them merged in - replaying them as a PATCH here
        // would target an issuance that was never persisted.
        return
      }

      reconcileCreatedRow(row.key, result.issuance)
      flashSaveStatus(row.key, "success")

      if (queued) {
        await persist({ ...merged, key: row.key, id: result.issuance.id, isDraft: false }, queued)
      }
      return
    }

    updateRow(row.key, { saveStatus: "saving" })
    const result = await updateIssuanceAction(
      row.id!,
      patch as z.infer<typeof updateIssuanceSchema.request>,
    )

    if ("error" in result) {
      toast.error(result.error)
      flashSaveStatus(row.key, "error")
      return
    }

    // Only `status` is taken from the response: it's the sole displayed field the server
    // derives on its own, and a fuller merge risks a late reply clobbering a cell the
    // user has edited since this save started.
    if (result.issuance) updateRow(row.key, { status: result.issuance.status })

    flashSaveStatus(row.key, "success")
  }

  function onCellClick(r: number, c: number) {
    activeCellSaveRef.current?.()
    activeCellSaveRef.current = null
    const editable = c !== CALCULATED_COLUMN_INDEX
    dispatch({ type: "CLICK_CELL", r, c, editable })
  }

  // Applies a patch optimistically and persists it. Shared by the cell-save path and by
  // inline account creation, which lands its patch long after the cell has moved on -
  // both need the accountClearedKeys bookkeeping, or the account cell stays flagged red.
  function applyPatch(row: IssuanceRow, patch: Partial<IssuanceRow>) {
    if (Object.keys(patch).length === 0) return

    if ("account_id" in patch) {
      setAccountClearedKeys((prev) => {
        const next = new Set(prev)
        if (patch.account_id === null) next.add(row.key)
        else next.delete(row.key)
        return next
      })
    }
    updateRow(row.key, patch)
    void persist(row, patch)
  }

  function onSaveAndMove(r: number, patch: Partial<IssuanceRow>, direction: Direction | null) {
    activeCellSaveRef.current = null
    const row = rows[r]
    if (!row) return

    applyPatch(row, patch)

    if (direction === null) {
      dispatch({ type: "EXIT_EDIT_ONLY" })
      return
    }

    const resolved = resolveNextCell(rows, interaction.sel, direction)
    if (resolved.appendRow) {
      setRows((prev) => [...prev, makeDraftRow(currentUserId)])
    }
    dispatch({ type: "MOVE", r: resolved.r, c: resolved.c, alsoEdit: interaction.sticky })
  }

  async function createAccountForRow(rowKey: string, name: string) {
    const row = rows.find((r) => r.key === rowKey)
    if (!row?.loyalty_program_id) return

    setPendingAccountLabels((prev) => ({ ...prev, [rowKey]: name }))
    updateRow(rowKey, { saveStatus: "saving" })

    const result = await createAccountForIssuanceAction(name, row.loyalty_program_id)

    setPendingAccountLabels((prev) => {
      const { [rowKey]: _removed, ...rest } = prev
      return rest
    })

    if ("error" in result) {
      toast.error(result.error)
      flashSaveStatus(rowKey, "error")
      return
    }

    setCreatedAccounts((prev) => [...prev, result.account])

    // Re-read the row: the await gave the operator time to keep editing, and applyPatch
    // hands the row it is given to persist() as the pre-patch base.
    const current = rowsRef.current.find((r) => r.key === rowKey)
    if (!current) return
    applyPatch(current, { account_id: result.account.id, account_nickname_id: result.nicknameId })
  }

  function onExitEdit() {
    dispatch({ type: "ESC" })
  }

  function registerActiveCellSave(save: (() => void) | null) {
    activeCellSaveRef.current = save
  }

  function requestSaveActiveCell() {
    activeCellSaveRef.current?.()
    activeCellSaveRef.current = null
  }

  async function onDeleteRow(r: number) {
    const row = rows[r]
    if (!row) return

    if (row.isDraft) {
      removeRow(row.key)
      return
    }

    const result = await deleteIssuanceAction(row.id!)
    if ("error" in result) {
      toast.error(result.error)
      return
    }
    removeRow(row.key)
  }

  function handleAddRow() {
    const lastRow = rows[rows.length - 1]
    if (lastRow && isUntouchedDraft(lastRow)) {
      dispatch({ type: "CLICK_CELL", r: rows.length - 1, c: 0, editable: true })
      return
    }

    const newIndex = rows.length
    setRows((prev) => [...prev, makeDraftRow(currentUserId)])
    dispatch({ type: "CLICK_CELL", r: newIndex, c: 0, editable: true })
  }

  function handleContainerKeyDown(e: React.KeyboardEvent<HTMLDivElement>) {
    if (interaction.editing) return

    if (e.key === "ArrowDown" || e.key === "ArrowUp" || e.key === "ArrowLeft" || e.key === "ArrowRight") {
      e.preventDefault()
      const dr = e.key === "ArrowDown" ? 1 : e.key === "ArrowUp" ? -1 : 0
      const dc = e.key === "ArrowRight" ? 1 : e.key === "ArrowLeft" ? -1 : 0
      const next = clampMove(rows.length, interaction.sel, dr, dc)
      dispatch({ type: "SELECT_ONLY", r: next.r, c: next.c })
      return
    }

    if (e.key === "Tab") {
      e.preventDefault()
      const direction: Direction = e.shiftKey ? "tab-left" : "tab-right"
      const resolved = resolveNextCell(rows, interaction.sel, direction)
      if (resolved.appendRow) setRows((prev) => [...prev, makeDraftRow(currentUserId)])
      dispatch({ type: "MOVE", r: resolved.r, c: resolved.c, alsoEdit: interaction.sticky })
      return
    }

    const isCalculated = interaction.sel.c === CALCULATED_COLUMN_INDEX
    if (isCalculated) return

    if (e.key === "Enter" || e.key === "F2") {
      e.preventDefault()
      dispatch({ type: "ENTER_EDIT" })
      return
    }

    if (e.key.length === 1 && !e.ctrlKey && !e.metaKey && !e.altKey) {
      dispatch({ type: "TYPE_REPLACE", char: e.key })
    }
  }

  return (
    <GridProvider
      value={{
        ...referenceData,
        // Overrides the prop: includes accounts created inline this session.
        accounts,
        rows,
        sel: interaction.sel,
        editing: interaction.editing,
        sticky: interaction.sticky,
        pendingChar: interaction.pendingChar,
        onCellClick,
        onSaveAndMove,
        onExitEdit,
        registerActiveCellSave,
        requestSaveActiveCell,
        onDeleteRow,
        createAccountForRow: (rowKey, name) => void createAccountForRow(rowKey, name),
        pendingAccountLabels,
      }}
    >
      <div className="sticky top-0 z-10 flex justify-end bg-background py-2">
        <Button type="button" size="sm" onClick={handleAddRow}>
          <Plus className="h-4 w-4" />
          Nova emissão
        </Button>
      </div>
      <div
        ref={containerRef}
        tabIndex={0}
        onKeyDown={handleContainerKeyDown}
        className="rounded-md border outline-none"
      >
        <Table>
          <TableHeader>
            <TableRow className="hover:bg-transparent">
              <TableHead className="w-8 p-1 text-center">#</TableHead>
              {COLUMN_LABELS.map((label, c) => (
                <TableHead
                  key={label}
                  className={cn("whitespace-nowrap", c === CALCULATED_COLUMN_INDEX && "text-right")}
                >
                  {label}
                </TableHead>
              ))}
              <TableHead className="w-8 p-1" />
            </TableRow>
          </TableHeader>
          <TableBody>
            {rows.map((row, r) => (
              <TableRow
                key={row.key}
                className={cn(
                  "group/row",
                  row.status === "cancelled" && "bg-red-50 hover:bg-red-50",
                  // The red stripe (on the number cell below) is what sets a failed save
                  // apart from a cancelled row, which shares this light-red background.
                  row.hasError && "bg-red-50 hover:bg-red-50",
                )}
              >
                <TableCell
                  className={cn(
                    "relative w-8 p-1 text-center text-xs text-muted-foreground tabular-nums",
                    row.hasError && "border-l-4 border-l-red-600",
                  )}
                >
                  {r + 1}
                  <span className="absolute right-0.5 top-0.5">
                    <AutosaveDot status={row.saveStatus} />
                  </span>
                </TableCell>
                {Array.from({ length: COLUMN_COUNT }, (_, c) => {
                  const accountNeedsPicking =
                    c === ACCOUNT_COLUMN_INDEX && accountClearedKeys.has(row.key)
                  return (
                    <TableCell
                      key={c}
                      // Background rather than a ring or border: the selection ring below
                      // already owns --tw-ring-color on this cell, and two competing
                      // ring-* utilities resolve by their order in the generated CSS, not
                      // by the order they appear here.
                      className={cn(
                        "whitespace-normal border-l p-0 first:border-l-0",
                        accountNeedsPicking && "bg-red-100",
                        interaction.sel.r === r &&
                          interaction.sel.c === c &&
                          "ring-2 ring-inset ring-primary",
                      )}
                      title={
                        accountNeedsPicking
                          ? row.isDraft
                            ? "Selecione uma conta — a emissão não será salva sem ela."
                            : "Selecione uma conta — a emissão ainda está vinculada à conta do programa anterior."
                          : undefined
                      }
                    >
                      <GridCell r={r} c={c} />
                    </TableCell>
                  )
                })}
                <TableCell className="p-1">
                  <RowActions r={r} />
                </TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      </div>
    </GridProvider>
  )
}

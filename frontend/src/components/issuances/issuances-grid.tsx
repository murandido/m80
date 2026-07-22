"use client"

import { useCallback, useEffect, useMemo, useReducer, useRef, useState } from "react"
import { Plus, Keyboard } from "lucide-react"
import { toast } from "sonner"
import { cn } from "@/lib/utils"
import { TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table"
import { Button } from "@/components/ui/button"
import { Popover, PopoverContent, PopoverTrigger } from "@/components/ui/popover"
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from "@/components/ui/alert-dialog"
import { GridContextValue, GridProvider, GridReferenceData } from "@/components/issuances/grid-context"
import { GridCell } from "@/components/issuances/grid-cell"
import { RowActions } from "@/components/issuances/row-actions"
import { AutosaveDot } from "@/components/issuances/autosave-dot"
import { interactionReducer, initialInteractionState } from "@/components/issuances/interaction-reducer"
import { IssuanceRow, issuanceToRow, makeDraftRow, draftIsReadyToCreate } from "@/components/issuances/types"
import { clearPatchForColumn } from "@/components/issuances/column-fields"
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

// One undoable step: the patch that restores a row to its prior state, keyed by the
// row's stable client id (not its index, which shifts as rows are added/removed).
interface HistoryEntry {
  key: string
  patch: Partial<IssuanceRow>
}

// Caps each history stack so a long editing session can't grow them without bound.
const HISTORY_LIMIT = 100

// Shown in the "Atalhos" popover so the keyboard-first workflow is discoverable.
const SHORTCUTS: { keys: string; label: string }[] = [
  { keys: "Tab / Shift+Tab", label: "Próxima / célula anterior" },
  { keys: "Enter / Shift+Enter", label: "Célula abaixo / acima" },
  { keys: "Setas", label: "Mover a seleção" },
  { keys: "F2 ou digitar", label: "Editar a célula" },
  { keys: "Esc", label: "Cancelar a edição" },
  { keys: "Home / End", label: "Início / fim da linha" },
  { keys: "Ctrl+Home / Ctrl+End", label: "Início / fim da grade" },
  { keys: "Ctrl + setas", label: "Saltar até a borda" },
  { keys: "Ctrl+Enter", label: "Nova emissão" },
  { keys: "Delete", label: "Limpar a célula" },
  { keys: "Ctrl+C / Ctrl+V", label: "Copiar / colar" },
  { keys: "Ctrl+Z / Ctrl+Shift+Z", label: "Desfazer / refazer" },
  { keys: "Ctrl+Shift+⌫", label: "Excluir a linha" },
  { keys: "/", label: 'Separar "programa/cia" (Cia aérea, Localizador)' },
]

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
  // A hidden but focusable textarea that holds focus while a cell is selected
  // (not editing). It exists so Ctrl+V fires a native `paste` event we can read
  // synchronously from clipboardData - a focused non-editable <div> never
  // receives paste events, and navigator.clipboard.readText() prompts for a
  // clipboard-read permission on first use.
  const pasteCatcherRef = useRef<HTMLTextAreaElement>(null)
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
  // Row index awaiting delete confirmation, driven by the Ctrl+Shift+Backspace shortcut.
  // Null means the confirm dialog is closed.
  const [pendingDeleteRow, setPendingDeleteRow] = useState<number | null>(null)
  // Lets the async account creation read the rows as they are when it resolves, not as
  // they were when it started.
  const rowsRef = useRef(rows)
  // Mirrors the interaction state for the stable (useCallback) handlers exposed through the
  // context: they read sel/sticky here instead of closing over the render's `interaction`,
  // so their identity stays stable and the context value doesn't change on every selection
  // move (which is what lets the memoized cells skip re-rendering).
  const interactionRef = useRef(interaction)
  // Undo/redo history. Each entry is the inverse patch (the field values as they were
  // before a change) plus the row it targets. Every value mutation funnels through
  // applyPatch, which records the inverse here; Ctrl+Z/Ctrl+Y replay entries between the
  // two stacks. Refs, not state: the history must not trigger re-renders and must be read
  // fresh inside the keydown handler without stale-closure surprises. historyModeRef tells
  // applyPatch which stack to feed while a replay is in flight (see runHistory).
  const undoStackRef = useRef<HistoryEntry[]>([])
  const redoStackRef = useRef<HistoryEntry[]>([])
  const historyModeRef = useRef<"record" | "undo" | "redo">("record")

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
    interactionRef.current = interaction
  }, [interaction])

  useEffect(() => {
    if (!interaction.editing) {
      // preventScroll: the catcher sits off-screen; focusing it must not yank
      // the viewport away from the cell the user is actually on.
      pasteCatcherRef.current?.focus({ preventScroll: true })
    }
  }, [interaction.editing, interaction.sel.r, interaction.sel.c])

  // Keeps the selected cell in view as the user navigates. The catcher holds focus
  // off-screen, so the browser never scrolls to the active cell on its own; without this,
  // arrow/Tab navigation to a cell outside the viewport would leave it hidden. `nearest`
  // scrolls the minimum on both axes, and scroll-mt on the cells clears the sticky header.
  useEffect(() => {
    const cell = containerRef.current?.querySelector(
      `[data-cell="${interaction.sel.r}-${interaction.sel.c}"]`,
    )
    cell?.scrollIntoView({ block: "nearest", inline: "nearest" })
  }, [interaction.sel.r, interaction.sel.c])

  // Every handler below is wrapped in useCallback and reads volatile state via refs
  // (rowsRef/interactionRef) or stable setters, so their identity never changes across
  // renders. That keeps the memoized context value stable, which is the whole point of the
  // split: the cells only re-render when their own props change, not on every keystroke.
  const updateRow = useCallback((key: string, patch: Partial<IssuanceRow>) => {
    setRows((prev) => prev.map((row) => (row.key === key ? { ...row, ...patch } : row)))
  }, [])

  const removeRow = useCallback(
    (key: string) => {
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
    },
    [currentUserId],
  )

  const flashSaveStatus = useCallback(
    (key: string, status: "success" | "error") => {
      // hasError outlives the flash: the dot resets to idle after 900ms, but a failed row
      // stays flagged red until a later save succeeds and clears it here.
      updateRow(key, { saveStatus: status, hasError: status === "error" })
      setTimeout(() => updateRow(key, { saveStatus: "idle" }), 900)
    },
    [updateRow],
  )

  // The backend fills defaults the client can't know (airline from the loyalty program,
  // the default agency, issuer from the JWT) and derives `status`, so a created row is
  // re-hydrated from the response instead of being patched with just the new id. The
  // draft's key is kept - issuanceToRow would swap in the backend id, remounting the row
  // and dropping the user's cell selection mid-typing.
  const reconcileCreatedRow = useCallback((key: string, issuance: Issuance) => {
    setRows((prev) =>
      prev.map((row) =>
        row.key === key ? { ...issuanceToRow(issuance), key, saveStatus: row.saveStatus } : row,
      ),
    )
  }, [])

  // persistRef holds the latest persist so the recursive replay (queued patch after a
  // create resolves) can call it without persist listing itself as a dependency.
  const persistRef = useRef<(row: IssuanceRow, patch: Partial<IssuanceRow>) => Promise<void>>(null)

  const persist = useCallback(
    async (row: IssuanceRow, patch: Partial<IssuanceRow>) => {
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
          await persistRef.current?.(
            { ...merged, key: row.key, id: result.issuance.id, isDraft: false },
            queued,
          )
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
    },
    [updateRow, flashSaveStatus, reconcileCreatedRow],
  )
  persistRef.current = persist

  const onCellClick = useCallback((r: number, c: number) => {
    activeCellSaveRef.current?.()
    activeCellSaveRef.current = null
    const editable = c !== CALCULATED_COLUMN_INDEX
    dispatch({ type: "CLICK_CELL", r, c, editable })
  }, [])

  // Applies a patch optimistically and persists it. Shared by the cell-save path and by
  // inline account creation, which lands its patch long after the cell has moved on -
  // both need the accountClearedKeys bookkeeping, or the account cell stays flagged red.
  const applyPatch = useCallback(
    (row: IssuanceRow, patch: Partial<IssuanceRow>) => {
      if (Object.keys(patch).length === 0) return

      // Record the inverse (the fields as they were before this patch) so Ctrl+Z can
      // replay it. `row` is the pre-patch state, so this is just its current values for
      // the keys being changed. During a replay, historyModeRef routes the inverse to the
      // opposite stack instead, which is what makes redo fall out for free.
      const inverse = Object.fromEntries(
        Object.keys(patch).map((key) => [key, row[key as keyof IssuanceRow]]),
      ) as Partial<IssuanceRow>
      if (historyModeRef.current === "record") {
        undoStackRef.current.push({ key: row.key, patch: inverse })
        if (undoStackRef.current.length > HISTORY_LIMIT) undoStackRef.current.shift()
        redoStackRef.current = [] // a fresh edit invalidates the redo stack
      } else if (historyModeRef.current === "undo") {
        redoStackRef.current.push({ key: row.key, patch: inverse })
      } else {
        undoStackRef.current.push({ key: row.key, patch: inverse })
      }

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
    },
    [updateRow, persist],
  )

  // Replays one step off the undo (or redo) stack. Re-applying through applyPatch both
  // persists the reversal and captures the current values onto the opposite stack, so
  // redo needs no special-casing. Entries whose row no longer exists (a deleted row -
  // structural changes are out of scope) are skipped so the stacks never dead-end. Reads
  // the current column from interactionRef, so it stays a stable identity like the rest.
  const runHistory = useCallback(
    (mode: "undo" | "redo") => {
      const stack = mode === "undo" ? undoStackRef : redoStackRef
      let entry = stack.current.pop()
      while (entry && !rowsRef.current.some((r) => r.key === entry!.key)) {
        entry = stack.current.pop()
      }
      if (!entry) return

      const row = rowsRef.current.find((r) => r.key === entry!.key)
      if (!row) return

      historyModeRef.current = mode
      applyPatch(row, entry.patch)
      historyModeRef.current = "record"

      // Move the selection onto the affected row (keeping the current column) so the change
      // the user just undid or redid is on screen rather than happening off in another row.
      const rowIndex = rowsRef.current.findIndex((r) => r.key === entry!.key)
      if (rowIndex >= 0) dispatch({ type: "SELECT_ONLY", r: rowIndex, c: interactionRef.current.sel.c })
    },
    [applyPatch],
  )

  const onSaveAndMove = useCallback(
    (r: number, patch: Partial<IssuanceRow>, direction: Direction | null) => {
      activeCellSaveRef.current = null
      const currentRows = rowsRef.current
      const row = currentRows[r]
      if (!row) return

      applyPatch(row, patch)

      if (direction === null) {
        dispatch({ type: "EXIT_EDIT_ONLY" })
        return
      }

      const { sel, sticky } = interactionRef.current
      const resolved = resolveNextCell(currentRows, sel, direction)
      if (resolved.appendRow) {
        setRows((prev) => [...prev, makeDraftRow(currentUserId)])
      }
      dispatch({ type: "MOVE", r: resolved.r, c: resolved.c, alsoEdit: sticky })
    },
    [applyPatch, currentUserId],
  )

  const createAccountForRow = useCallback(
    async (rowKey: string, name: string) => {
      const row = rowsRef.current.find((r) => r.key === rowKey)
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
    },
    [applyPatch, updateRow, flashSaveStatus],
  )

  const onExitEdit = useCallback(() => {
    dispatch({ type: "ESC" })
  }, [])

  const registerActiveCellSave = useCallback((save: (() => void) | null) => {
    activeCellSaveRef.current = save
  }, [])

  const requestSaveActiveCell = useCallback(() => {
    activeCellSaveRef.current?.()
    activeCellSaveRef.current = null
  }, [])

  const onDeleteRow = useCallback(
    async (r: number) => {
      const row = rowsRef.current[r]
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
    },
    [removeRow],
  )

  // Keyboard entry point for deleting the active row. An untouched empty draft has nothing
  // to lose, so it is dropped without a prompt; anything else opens the confirm dialog.
  function requestRowDelete(r: number) {
    const row = rows[r]
    if (!row) return
    if (isUntouchedDraft(row)) {
      void onDeleteRow(r)
      return
    }
    setPendingDeleteRow(r)
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

    // Power-user jumps and row ops, handled before the single-step arrows/Enter below so
    // the modifier combinations win over their unmodified counterparts.
    if ((e.ctrlKey || e.metaKey) && e.key === "Enter") {
      e.preventDefault()
      handleAddRow()
      return
    }
    if ((e.ctrlKey || e.metaKey) && e.shiftKey && (e.key === "Backspace" || e.key === "Delete")) {
      e.preventDefault()
      requestRowDelete(interaction.sel.r)
      return
    }
    if (
      e.key === "Home" ||
      e.key === "End" ||
      ((e.ctrlKey || e.metaKey) &&
        (e.key === "ArrowUp" ||
          e.key === "ArrowDown" ||
          e.key === "ArrowLeft" ||
          e.key === "ArrowRight"))
    ) {
      e.preventDefault()
      const lastRow = Math.max(rows.length - 1, 0)
      const lastCol = COLUMN_COUNT - 1
      const toCorner = e.ctrlKey || e.metaKey // Ctrl+Home/End jump to the grid corner
      let r = interaction.sel.r
      let c = interaction.sel.c
      if (e.key === "Home") {
        c = 0
        if (toCorner) r = 0
      } else if (e.key === "End") {
        c = lastCol
        if (toCorner) r = lastRow
      } else if (e.key === "ArrowUp") r = 0
      else if (e.key === "ArrowDown") r = lastRow
      else if (e.key === "ArrowLeft") c = 0
      else if (e.key === "ArrowRight") c = lastCol
      dispatch({ type: "SELECT_ONLY", r, c })
      return
    }

    // Enter moves down and Shift+Enter up, like a spreadsheet; editing a cell is F2,
    // a click, or typing (type-to-replace). Handled with the arrows so it works from any
    // column, including the read-only calculated one.
    if (
      e.key === "ArrowDown" ||
      e.key === "ArrowUp" ||
      e.key === "ArrowLeft" ||
      e.key === "ArrowRight" ||
      e.key === "Enter"
    ) {
      e.preventDefault()
      const dr =
        e.key === "ArrowDown" || (e.key === "Enter" && !e.shiftKey)
          ? 1
          : e.key === "ArrowUp" || (e.key === "Enter" && e.shiftKey)
            ? -1
            : 0
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

    // Undo/redo are grid-wide, not tied to the active cell, so they sit before the
    // calculated-column guard below (which only gates per-cell editing actions).
    if ((e.ctrlKey || e.metaKey) && e.key.toLowerCase() === "z" && !e.shiftKey) {
      e.preventDefault()
      runHistory("undo")
      return
    }
    if (
      (e.ctrlKey || e.metaKey) &&
      (e.key.toLowerCase() === "y" || (e.shiftKey && e.key.toLowerCase() === "z"))
    ) {
      e.preventDefault()
      runHistory("redo")
      return
    }

    const isCalculated = interaction.sel.c === CALCULATED_COLUMN_INDEX
    if (isCalculated) return

    if (e.key === "F2") {
      e.preventDefault()
      dispatch({ type: "ENTER_EDIT" })
      return
    }

    if (e.key === "Delete") {
      const patch = clearPatchForColumn(interaction.sel.c)
      const row = rows[interaction.sel.r]
      if (patch && row) {
        e.preventDefault()
        applyPatch(row, patch)
      }
      return
    }

    if ((e.ctrlKey || e.metaKey) && e.key.toLowerCase() === "c") {
      e.preventDefault()
      void copyActiveCell()
      return
    }

    // Ctrl+V is not handled here: it arrives as a native `paste` event on the
    // focused catcher textarea (see handlePaste), which reads the clipboard
    // synchronously without a permission prompt.

    if (e.key.length === 1 && !e.ctrlKey && !e.metaKey && !e.altKey) {
      dispatch({ type: "TYPE_REPLACE", char: e.key })
    }
  }

  // Copies the value the user actually sees (a combobox shows a name, not its id),
  // read from the rendered read-view via the cell's data-cell marker. The empty
  // placeholder ("—") copies as an empty string.
  async function copyActiveCell() {
    const node = containerRef.current?.querySelector(
      `[data-cell="${interaction.sel.r}-${interaction.sel.c}"]`,
    )
    const text = (node?.textContent ?? "").trim()
    const copyable = text === "—" ? "" : text
    try {
      await navigator.clipboard.writeText(copyable)
      toast.success("Célula copiada para a área de transferência")
    } catch {
      toast.error("Não foi possível copiar a célula")
    }
  }

  // Pastes into the active cell by entering edit mode with the clipboard text
  // pre-filled (pendingChar carries multi-character text just as well as a single
  // typed char), so each cell's own parser decides the value on commit. Fires
  // from the catcher textarea's native paste event, so it never touches the
  // permission-gated navigator.clipboard.readText().
  function handlePaste(e: React.ClipboardEvent<HTMLTextAreaElement>) {
    // Only meaningful outside edit mode: while editing, the cell's own input has
    // focus and handles its native paste directly.
    if (interaction.editing) return
    if (interaction.sel.c === CALCULATED_COLUMN_INDEX) return
    e.preventDefault()
    const text = e.clipboardData.getData("text/plain")
    if (text) dispatch({ type: "TYPE_REPLACE", char: text })
  }

  // The context carries only referentially-stable data: reference lists, the memoized
  // callbacks above and pendingAccountLabels (which changes only during inline account
  // creation). The volatile per-cell state (row/isEditing/pendingChar) is passed as props
  // in the render loop instead, so moving the selection doesn't invalidate this value and
  // the memoized cells can skip re-rendering. Depends on the individual reference arrays,
  // not the `referenceData` rest object, which is a fresh object every render.
  const contextValue = useMemo<GridContextValue>(
    () => ({
      loyaltyPrograms: referenceData.loyaltyPrograms,
      airlines: referenceData.airlines,
      // Overrides the prop: includes accounts created inline this session.
      accounts,
      agencies: referenceData.agencies,
      users: referenceData.users,
      people: referenceData.people,
      airports: referenceData.airports,
      onCellClick,
      onSaveAndMove,
      onExitEdit,
      registerActiveCellSave,
      requestSaveActiveCell,
      onDeleteRow,
      createAccountForRow,
      pendingAccountLabels,
    }),
    [
      referenceData.loyaltyPrograms,
      referenceData.airlines,
      accounts,
      referenceData.agencies,
      referenceData.users,
      referenceData.people,
      referenceData.airports,
      onCellClick,
      onSaveAndMove,
      onExitEdit,
      registerActiveCellSave,
      requestSaveActiveCell,
      onDeleteRow,
      createAccountForRow,
      pendingAccountLabels,
    ],
  )

  return (
    <GridProvider value={contextValue}>
      <div className="sticky top-0 z-10 flex items-center justify-end gap-2 bg-background py-2">
        <Popover>
          <PopoverTrigger asChild>
            <Button type="button" variant="outline" size="sm">
              <Keyboard className="h-4 w-4" />
              Atalhos
            </Button>
          </PopoverTrigger>
          <PopoverContent align="end" className="w-80">
            <p className="mb-2 text-sm font-medium">Atalhos de teclado</p>
            <dl className="space-y-1">
              {SHORTCUTS.map((shortcut) => (
                <div
                  key={shortcut.keys}
                  className="flex items-baseline justify-between gap-3 text-xs"
                >
                  <dt className="text-muted-foreground">{shortcut.label}</dt>
                  <dd className="shrink-0 font-mono text-[11px]">{shortcut.keys}</dd>
                </div>
              ))}
            </dl>
          </PopoverContent>
        </Popover>
        <Button type="button" size="sm" onClick={handleAddRow}>
          <Plus className="h-4 w-4" />
          Nova emissão
        </Button>
      </div>
      <div
        ref={containerRef}
        onKeyDown={handleContainerKeyDown}
        // Bounded scroll region so the header can stick to its top (a sticky header needs a
        // scroll ancestor with a fixed height) and long lists scroll inside the grid.
        className="relative max-h-[calc(100vh-13rem)] overflow-auto rounded-md border outline-none"
      >
        {/* Off-screen focus holder: keydown and paste events from it bubble to the
            container's onKeyDown/handlePaste. Kept empty (onInput clears any char
            that type-to-replace lets through) so it never accumulates text. */}
        <textarea
          ref={pasteCatcherRef}
          aria-hidden="true"
          tabIndex={-1}
          onPaste={handlePaste}
          onInput={(e) => {
            e.currentTarget.value = ""
          }}
          className="pointer-events-none absolute left-0 top-0 h-px w-px resize-none opacity-0"
        />
        <table className="w-full caption-bottom text-sm">
          <TableHeader>
            <TableRow className="hover:bg-transparent">
              <TableHead className="sticky top-0 z-10 w-8 bg-background p-1 text-center">#</TableHead>
              {COLUMN_LABELS.map((label, c) => (
                <TableHead
                  key={label}
                  className={cn(
                    "sticky top-0 z-10 whitespace-nowrap bg-background",
                    c === CALCULATED_COLUMN_INDEX && "text-right",
                  )}
                >
                  {label}
                </TableHead>
              ))}
              <TableHead className="sticky top-0 z-10 w-8 bg-background p-1" />
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
                  const isSelected = interaction.sel.r === r && interaction.sel.c === c
                  const isEditing = isSelected && interaction.editing
                  return (
                    <TableCell
                      key={c}
                      // Marks the cell for Ctrl+C, which copies its rendered text.
                      data-cell={`${r}-${c}`}
                      // Background rather than a ring or border: the selection ring below
                      // already owns --tw-ring-color on this cell, and two competing
                      // ring-* utilities resolve by their order in the generated CSS, not
                      // by the order they appear here.
                      className={cn(
                        // scroll-mt clears the sticky header when scrollIntoView lands here.
                        "scroll-mt-9 whitespace-normal border-l p-0 first:border-l-0",
                        accountNeedsPicking && "bg-red-100",
                        isSelected && "ring-2 ring-inset ring-primary",
                      )}
                      title={
                        accountNeedsPicking
                          ? row.isDraft
                            ? "Selecione uma conta — a emissão não será salva sem ela."
                            : "Selecione uma conta — a emissão ainda está vinculada à conta do programa anterior."
                          : undefined
                      }
                    >
                      {/* Volatile state as props: `row` (stable ref unless this row was
                          edited), whether this cell is editing, and pendingChar only while
                          it is. Every other cell gets identical props across a selection
                          move, so its memoized GridCell skips re-rendering. */}
                      <GridCell
                        r={r}
                        c={c}
                        row={row}
                        isEditing={isEditing}
                        pendingChar={isEditing ? interaction.pendingChar : null}
                      />
                    </TableCell>
                  )
                })}
                <TableCell className="p-1">
                  <RowActions r={r} />
                </TableCell>
              </TableRow>
            ))}
          </TableBody>
        </table>
      </div>

      {/* Confirmation for the keyboard row-delete (Ctrl+Shift+Backspace). The per-row
          trash button in RowActions has its own trigger-based dialog; this one is opened
          from the keyboard, so it is controlled and returns focus to the catcher on close. */}
      <AlertDialog
        open={pendingDeleteRow !== null}
        onOpenChange={(open) => {
          if (!open) setPendingDeleteRow(null)
        }}
      >
        <AlertDialogContent
          onCloseAutoFocus={(e) => {
            e.preventDefault()
            pasteCatcherRef.current?.focus({ preventScroll: true })
          }}
        >
          <AlertDialogHeader>
            <AlertDialogTitle>Deletar emissão</AlertDialogTitle>
            <AlertDialogDescription>Você realmente deseja deletar essa emissão?</AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel className="border-red-300 text-red-600 hover:bg-red-50 hover:text-red-700">
              Não
            </AlertDialogCancel>
            <AlertDialogAction
              onClick={() => {
                if (pendingDeleteRow !== null) void onDeleteRow(pendingDeleteRow)
                setPendingDeleteRow(null)
              }}
            >
              Sim, deletar
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </GridProvider>
  )
}

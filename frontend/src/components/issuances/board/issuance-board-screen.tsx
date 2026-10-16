"use client"

import { useEffect, useRef, useState } from "react"
import { AlertTriangle } from "lucide-react"
import { cn } from "@/lib/utils"
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert"
import type { GridReferenceData } from "@/components/issuances/grid-context"
import {
  buildCalculo,
  buildEmissaoResumo,
} from "@/components/issuances/emissao-message"
import { copyEmissaoText } from "@/components/issuances/copy-emissao-menu"
import { RedemptionFeeSummary } from "@/components/issuances/redemption-fee-summary"
import { IssuanceBoard } from "@/components/issuances/board/issuance-board"
import { IssuancePanel } from "@/components/issuances/board/issuance-panel"
import {
  BoardActions,
  BoardFilters,
  type FilterState,
} from "@/components/issuances/board/board-toolbar"
import { useIssuanceBoard } from "@/components/issuances/board/use-issuance-board"
import layout from "@/components/issuances/board/board-layout.module.css"
import type { Issuance } from "@/schemas/issuance"

interface IssuanceBoardScreenProps {
  initialIssuances: Issuance[]
  filterQuery: string
  initialFilters: FilterState
  hasSearch: boolean
  currentUserId: string | null
  referenceData: GridReferenceData
  // Some picker list failed to load; said on screen rather than shown as a short list.
  referenceListsFailed: boolean
}

// The issuances screen as cards and a side panel. Renders a fragment on purpose: its
// pieces are cells of the page's grid (board-layout.module.css), next to the title and the
// search box the page renders itself.
export function IssuanceBoardScreen({
  initialIssuances,
  filterQuery,
  initialFilters,
  hasSearch,
  currentUserId,
  referenceData,
  referenceListsFailed,
}: IssuanceBoardScreenProps) {
  const board = useIssuanceBoard({
    initialIssuances,
    currentUserId,
    filterQuery,
    referenceData,
  })
  const [openKey, setOpenKey] = useState<string | null>(null)
  // Whether the panel was opened to type a new issuance, which starts in its first field.
  const [openedForTyping, setOpenedForTyping] = useState(false)
  const [deleteRequested, setDeleteRequested] = useState(false)

  const openIndex =
    openKey === null ? -1 : board.rows.findIndex((row) => row.key === openKey)
  const openRow = openIndex === -1 ? undefined : board.rows[openIndex]
  // A row that left the list (deleted here or elsewhere) closes its panel.
  const panelOpen = openRow !== undefined

  // The live refresh protects the row being typed into from being rebuilt under the operator.
  useEffect(() => {
    // eslint-disable-next-line react-hooks/immutability -- hands the open row to the hook's live refresh, which reads it outside React
    board.openKeyRef.current = panelOpen ? openKey : null
  }, [board.openKeyRef, openKey, panelOpen])

  // The page behind a full-screen panel must not scroll on a phone.
  useEffect(() => {
    if (!panelOpen) return
    const media = window.matchMedia("(max-width: 1023px)")
    if (!media.matches) return
    const previous = document.body.style.overflow
    document.body.style.overflow = "hidden"
    return () => {
      document.body.style.overflow = previous
    }
  }, [panelOpen])

  function openCard(key: string) {
    setOpenedForTyping(false)
    setOpenKey(key)
  }

  function startNewIssuance() {
    // Whatever field is mid-edit saves on its blur before the panel switches rows.
    ;(document.activeElement as HTMLElement | null)?.blur()
    const key = board.addDraft()
    setOpenedForTyping(true)
    setOpenKey(key)
  }

  const lastOpenKeyRef = useRef<string | null>(null)
  function minimize() {
    lastOpenKeyRef.current = openKey
    ;(document.activeElement as HTMLElement | null)?.blur()
    setOpenKey(null)
  }

  // Back to the card that was open, so the arrows carry on from there. After the close has
  // rendered: the panel unmounting takes the focus with it.
  useEffect(() => {
    if (openKey !== null || !lastOpenKeyRef.current) return
    const key = lastOpenKeyRef.current
    lastOpenKeyRef.current = null
    document
      .querySelector<HTMLElement>(`[data-card-key="${CSS.escape(key)}"]`)
      ?.focus()
  }, [openKey])

  // Shortcuts that work wherever the focus is on this screen.
  const boardRef = useRef(board)
  // eslint-disable-next-line react-hooks/refs -- latest-ref so the keydown listener is bound once and still sees the current board
  boardRef.current = board
  const openRowRef = useRef(openRow)
  // eslint-disable-next-line react-hooks/refs -- latest-ref, same as boardRef above
  openRowRef.current = openRow
  useEffect(() => {
    function handleKeyDown(event: KeyboardEvent) {
      const ctrl = event.ctrlKey || event.metaKey
      if (!ctrl) return
      const row = openRowRef.current
      const { loyaltyPrograms, airlines, accounts, agencies, people } =
        boardRef.current

      if (event.key === "Enter" && !event.altKey && !event.shiftKey) {
        event.preventDefault()
        startNewIssuance()
      } else if (event.altKey && event.code === "KeyC" && row) {
        event.preventDefault()
        void copyEmissaoText(
          buildEmissaoResumo(row, {
            loyaltyPrograms,
            airlines,
            accounts,
            agencies,
            people,
          }),
          "resumo",
        )
      } else if (event.altKey && event.code === "KeyX" && row) {
        event.preventDefault()
        void copyEmissaoText(buildCalculo(row), "cálculo")
      } else if (event.shiftKey && event.key === "Backspace" && row) {
        event.preventDefault()
        setDeleteRequested(true)
      }
    }
    window.addEventListener("keydown", handleKeyDown)
    return () => window.removeEventListener("keydown", handleKeyDown)
    // startNewIssuance only reads refs and stable setters.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  return (
    <>
      <div className={layout.filters}>
        <BoardFilters
          loyaltyPrograms={board.loyaltyPrograms}
          airlines={board.airlines}
          accounts={board.accounts}
          agencies={board.agencies}
          people={board.people}
          users={board.users}
          initialFilters={initialFilters}
          hasSearch={hasSearch}
          currentUserId={currentUserId}
        />
      </div>
      <div className={layout.actions}>
        <BoardActions filterQuery={filterQuery} />
      </div>
      <hr className={cn(layout.separator, "border-slate-200")} />

      <div className={cn(layout.board, "flex flex-col gap-3")}>
        {referenceListsFailed && (
          <Alert>
            <AlertTriangle />
            <AlertTitle>Algumas listas não carregaram</AlertTitle>
            <AlertDescription>
              Companhias, contas, agências ou emissores podem aparecer
              incompletos nos campos de seleção. Recarregue a página para tentar
              de novo.
            </AlertDescription>
          </Alert>
        )}
        {board.rows.length === 0 && (
          <p className="text-muted-foreground text-sm">
            Nenhuma emissão no período e filtros escolhidos. Use o + ou
            Ctrl+Enter para cadastrar.
          </p>
        )}
        <IssuanceBoard
          rows={board.rows}
          loyaltyPrograms={board.loyaltyPrograms}
          agencies={board.agencies}
          users={board.users}
          openKey={panelOpen ? openKey : null}
          busyKeys={{ ...board.syncingRowKeys, ...board.lookingUpKeys }}
          onOpen={openCard}
          onAdd={startNewIssuance}
        />
        <RedemptionFeeSummary rows={board.rows} />
      </div>

      {openRow && (
        <aside
          aria-label={`Emissão ${openIndex + 1}`}
          className={cn(
            layout.panel,
            // A phone gets the whole screen; a desktop a panel beside the cards that stays
            // in view while the list scrolls.
            "fixed inset-0 z-40 overflow-y-auto bg-slate-200",
            // The full height of the screen, whatever the form holds: the header (3rem)
            // and the page's padding above and below are what is left out.
            "lg:sticky lg:inset-auto lg:top-8 lg:z-auto lg:h-[calc(100svh-7rem)] lg:rounded-2xl",
          )}
        >
          <IssuancePanel
            key={openRow.key}
            board={board}
            row={openRow}
            number={openIndex + 1}
            focusFirstField={openedForTyping}
            onMinimize={minimize}
            deleteRequested={deleteRequested}
            onDeleteRequestedChange={setDeleteRequested}
          />
        </aside>
      )}
    </>
  )
}

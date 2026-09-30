"use client"

import { useState } from "react"
import { ChevronDown, Merge, Trash2, TriangleAlert } from "lucide-react"
import { Button } from "@/components/ui/button"
import { cn } from "@/lib/utils"
import { AccountHolderRow as AccountHolderRowData } from "@/schemas/account"
import { PersonOption } from "@/schemas/person"
import { LoyaltyProgram } from "@/schemas/loyalty-program"
import { LoyaltyProgramBadge } from "./loyalty-program-badge"
import { AccountProgramBlock } from "./account-program-block"
import { AccountHolderDetails } from "./account-holder-details"
import { CreateAccountForm } from "./create-account-form"
import { MergeAccountHolderPanel } from "./merge-account-holder-panel"
import { DeleteHolderPanel } from "./delete-holder-panel"

interface AccountHolderRowProps {
  row: AccountHolderRowData
  people: PersonOption[]
  loyaltyPrograms: LoyaltyProgram[]
  canMerge: boolean
}

// One row of the accounts screen. To the operator a holder and their loyalty accounts are
// one thing - "Fulana's account" - so the row shows who, and a badge per program; opening
// it shows each program's credentials and, underneath, the holder's own data.
export function AccountHolderRow({
  row,
  people,
  loyaltyPrograms,
  canMerge,
}: AccountHolderRowProps) {
  // One panel at a time: they all slide out of the header's bottom edge.
  const [openPanel, setOpenPanel] = useState<
    "none" | "details" | "merge" | "delete"
  >("none")
  const isOpen = openPanel === "details"
  const isMergeOpen = openPanel === "merge"
  const isDeleteOpen = openPanel === "delete"

  const { holder, accounts } = row
  const aliases = holder.nicknames.map((nickname) => nickname.nickname)
  const programIds = accounts.map((account) => account.loyalty_program_id)
  // One administrator per holder is kept by the API and the bot, not by a constraint, so a
  // disagreement is still worth a badge if one ever slips through.
  const administratorIds = [
    ...new Set(
      accounts
        .map((account) => account.administrator_id)
        .filter((id): id is string => Boolean(id)),
    ),
  ]
  const hasAdminConflict = administratorIds.length > 1
  const isIncomplete =
    !holder.complete || accounts.some((account) => !account.complete)

  return (
    <div className="flex flex-col">
      <div
        className={cn(
          "flex items-center gap-2 border border-slate-200 pr-4 transition-colors duration-200",
          openPanel === "none" ? "rounded-md" : "rounded-t-md bg-slate-50",
        )}
      >
        <button
          type="button"
          onClick={() =>
            setOpenPanel((prev) => (prev === "details" ? "none" : "details"))
          }
          aria-expanded={isOpen}
          className="flex min-w-0 flex-1 items-center justify-between gap-3 py-3 pl-4 text-left"
        >
          <div className="flex min-w-0 flex-col gap-1">
            <div className="flex flex-wrap items-center gap-2">
              <span className="text-sm font-medium text-slate-800">
                {holder.name}
              </span>
              {hasAdminConflict && (
                <span
                  className="inline-flex items-center gap-1 rounded-md bg-amber-100 px-2 py-0.5 text-xs font-medium text-amber-800"
                  title="As contas deste titular têm administradores diferentes. Escolha um na seção Titular."
                >
                  <TriangleAlert className="h-3 w-3" />
                  Administradores diferentes
                </span>
              )}
              {isIncomplete && (
                <span className="rounded-md bg-slate-100 px-2 py-0.5 text-xs text-slate-600">
                  Incompleto
                </span>
              )}
            </div>
            <div className="flex flex-wrap items-center gap-1.5">
              {accounts.length === 0 ? (
                <span className="text-xs text-slate-500">Nenhum programa</span>
              ) : (
                accounts.map((account) => (
                  <LoyaltyProgramBadge
                    key={account.id}
                    loyaltyProgram={account.loyalty_program}
                  />
                ))
              )}
            </div>
            {aliases.length > 0 && (
              <span className="truncate text-xs text-slate-500">
                Apelidos: {aliases.join(", ")}
              </span>
            )}
          </div>
          <ChevronDown
            className={cn(
              "h-4 w-4 shrink-0 text-slate-500 transition-transform duration-200",
              isOpen && "rotate-180",
            )}
          />
        </button>
        {canMerge && (
          <Button
            variant="outline"
            size="icon"
            title="Mesclar outra linha nesta"
            aria-label="Mesclar outra linha nesta"
            onClick={() =>
              setOpenPanel((prev) => (prev === "merge" ? "none" : "merge"))
            }
            className={cn(
              "shrink-0 transition-colors duration-200",
              isMergeOpen
                ? "border-amber-600 bg-amber-600 text-white hover:border-amber-700 hover:bg-amber-700"
                : "border-slate-800 text-slate-800 hover:border-amber-400 hover:bg-amber-400 hover:text-white active:border-amber-800 active:bg-amber-800 active:text-white",
            )}
          >
            <Merge className="h-4 w-4" />
          </Button>
        )}
        {/* The whole row goes: the holder and every account of theirs, deleted where nothing
            refers to them and archived otherwise. The panel says which before it happens. */}
        <Button
          variant="outline"
          size="icon"
          title="Excluir esta linha"
          aria-label="Excluir esta linha"
          onClick={() =>
            setOpenPanel((prev) => (prev === "delete" ? "none" : "delete"))
          }
          className={cn(
            "shrink-0 transition-colors duration-200",
            isDeleteOpen
              ? "border-red-600 bg-red-600 text-white hover:border-red-700 hover:bg-red-700"
              : "border-slate-800 text-slate-800 hover:border-red-600 hover:bg-red-600 hover:text-white active:border-red-950 active:bg-red-950 active:text-white",
          )}
        >
          <Trash2 className="h-4 w-4" />
        </Button>
      </div>

      <div
        className={cn(
          "grid transition-all duration-300 ease-in-out",
          isOpen ? "grid-rows-[1fr]" : "grid-rows-[0fr]",
        )}
      >
        <div className="overflow-hidden">
          <div className="flex flex-col gap-4 rounded-b-md border border-t-0 border-slate-200 bg-slate-50 p-4">
            <section className="flex flex-col gap-2">
              <h3 className="text-xs font-semibold tracking-wide text-slate-500 uppercase">
                Programas
              </h3>
              {accounts.length === 0 && (
                <p className="text-sm text-slate-500">
                  Nenhuma conta de programa ainda.
                </p>
              )}
              {accounts.map((account) => (
                <AccountProgramBlock
                  key={account.id}
                  account={account}
                  people={people}
                  loyaltyPrograms={loyaltyPrograms}
                  canReconcile={canMerge}
                />
              ))}
              <CreateAccountForm
                people={people}
                // A holder has one account per program: only the ones they lack are offered.
                loyaltyPrograms={loyaltyPrograms.filter(
                  (program) => !programIds.includes(program.id),
                )}
                holderId={holder.id}
              />
            </section>

            <section className="flex flex-col gap-2">
              <h3 className="text-xs font-semibold tracking-wide text-slate-500 uppercase">
                Titular
              </h3>
              <AccountHolderDetails
                holder={holder}
                people={people}
                administratorIds={administratorIds}
                hasAccounts={accounts.length > 0}
              />
            </section>
          </div>
        </div>
      </div>

      {canMerge && (
        <MergeAccountHolderPanel
          holder={holder}
          people={people}
          isOpen={isMergeOpen}
          onMerged={() => setOpenPanel("none")}
        />
      )}

      <DeleteHolderPanel
        holderId={holder.id}
        holderName={holder.name}
        isOpen={isDeleteOpen}
        onCancel={() => setOpenPanel("none")}
      />
    </div>
  )
}

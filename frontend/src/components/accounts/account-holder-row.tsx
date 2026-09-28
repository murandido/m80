"use client"

import { useState } from "react"
import { ChevronDown, Merge, TriangleAlert } from "lucide-react"
import { Button } from "@/components/ui/button"
import { cn } from "@/lib/utils"
import {
  AccountHolderRow as AccountHolderRowData,
  AccountOption,
} from "@/schemas/account"
import { PersonOption } from "@/schemas/person"
import { LoyaltyProgram } from "@/schemas/loyalty-program"
import { LoyaltyProgramBadge } from "./loyalty-program-badge"
import { AccountProgramBlock } from "./account-program-block"
import { AccountHolderDetails } from "./account-holder-details"
import { CreateAccountForm } from "./create-account-form"
import { MergeAccountHolderPanel } from "./merge-account-holder-panel"

interface AccountHolderRowProps {
  row: AccountHolderRowData
  people: PersonOption[]
  loyaltyPrograms: LoyaltyProgram[]
  // Accounts nobody holds, for the merge picker. Empty for anyone who cannot merge, so the
  // list is not shipped to browsers that have no use for it.
  orphanAccounts: AccountOption[]
  canMerge: boolean
}

// The label a row is known by. Same fallback order as accountLabel in account-combobox.tsx
// for a row with no holder, so one account does not read as two different things depending
// on the screen.
export function holderRowTitle(row: AccountHolderRowData): string {
  if (row.holder) return row.holder.name
  const account = row.accounts[0]
  return (
    account?.login ?? account?.nicknames[0]?.nickname ?? "Conta sem titular"
  )
}

// One row of the accounts screen. To the operator a holder and their loyalty accounts are
// one thing - "Fulana's account" - so the row shows who, and a badge per program; opening
// it shows each program's credentials and, underneath, the holder's own data.
export function AccountHolderRow({
  row,
  people,
  loyaltyPrograms,
  orphanAccounts,
  canMerge,
}: AccountHolderRowProps) {
  // One panel at a time: both slide out of the header's bottom edge.
  const [openPanel, setOpenPanel] = useState<"none" | "details" | "merge">(
    "none",
  )
  const isOpen = openPanel === "details"
  const isMergeOpen = openPanel === "merge"

  const title = holderRowTitle(row)
  // Only a holderless row can be archived: a holder's archived accounts are not listed.
  const isArchived = !row.holder && Boolean(row.accounts[0]?.deleted_at)
  // The holder's list is the row's list; a holderless account only has its own.
  const aliases = (
    row.holder?.nicknames ??
    row.accounts[0]?.nicknames ??
    []
  ).map((nickname) => nickname.nickname)
  const isIncomplete =
    (row.holder && !row.holder.complete) ||
    row.accounts.some((account) => !account.complete)

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
                {title}
              </span>
              {/* The NOT NULL to-do: every holderless account, archived or not, has to be
                  given a holder before accounts.holder_id can be made mandatory. */}
              {!row.holder && (
                <span
                  className="inline-flex items-center gap-1 rounded-md bg-amber-100 px-2 py-0.5 text-xs font-medium text-amber-800"
                  title="Sem titular, esta conta impede que o titular vire obrigatório no banco."
                >
                  <TriangleAlert className="h-3 w-3" />
                  Precisa de titular
                </span>
              )}
              {isArchived && (
                <span className="rounded-md bg-slate-200 px-2 py-0.5 text-xs text-slate-700">
                  Arquivada
                </span>
              )}
              {isIncomplete && (
                <span className="rounded-md bg-slate-100 px-2 py-0.5 text-xs text-slate-600">
                  Incompleto
                </span>
              )}
            </div>
            <div className="flex flex-wrap items-center gap-1.5">
              {row.accounts.length === 0 ? (
                <span className="text-xs text-slate-500">Nenhum programa</span>
              ) : (
                row.accounts.map((account) => (
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
        {canMerge && row.holder && (
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
              {row.accounts.length === 0 && (
                <p className="text-sm text-slate-500">
                  Nenhuma conta de programa ainda.
                </p>
              )}
              {row.accounts.map((account) => (
                <AccountProgramBlock
                  key={account.id}
                  account={account}
                  people={people}
                  loyaltyPrograms={loyaltyPrograms}
                  canReconcile={canMerge}
                />
              ))}
              {row.holder && (
                <CreateAccountForm
                  people={people}
                  loyaltyPrograms={loyaltyPrograms}
                  holderId={row.holder.id}
                />
              )}
            </section>

            <section className="flex flex-col gap-2">
              <h3 className="text-xs font-semibold tracking-wide text-slate-500 uppercase">
                Titular
              </h3>
              {row.holder ? (
                <AccountHolderDetails
                  holder={row.holder}
                  canDelete={row.accounts.length === 0}
                />
              ) : (
                <p className="text-sm text-amber-800">
                  {isArchived
                    ? // An archived account cannot be edited, so merging is the only way in.
                      "Conta arquivada sem titular. Um administrador resolve na linha do titular certo, em Mesclar."
                    : `Esta conta não tem titular. Escolha um em "Editar conta"${
                        canMerge
                          ? " ou, na linha do titular certo, use Mesclar para trazê-la"
                          : ""
                      }.`}
                </p>
              )}
            </section>
          </div>
        </div>
      </div>

      {canMerge && row.holder && (
        <MergeAccountHolderPanel
          holder={row.holder}
          people={people}
          orphanAccounts={orphanAccounts}
          loyaltyPrograms={loyaltyPrograms}
          isOpen={isMergeOpen}
          onMerged={() => setOpenPanel("none")}
        />
      )}
    </div>
  )
}

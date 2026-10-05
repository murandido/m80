"use client"

import { useState } from "react"
import { useRouter } from "next/navigation"
import { Eye, EyeOff, Menu, Trash2, Users } from "lucide-react"
import { Button } from "@/components/ui/button"
import { cn } from "@/lib/utils"
import { Account } from "@/schemas/account"
import { PersonOption } from "@/schemas/person"
import { LoyaltyProgram } from "@/schemas/loyalty-program"
import { LoyaltyProgramBadge } from "./loyalty-program-badge"
import { AccountCategoryBadge } from "./account-category-badge"
import { RedemptionFeeWaiverBadge } from "./redemption-fee-waiver-badge"
import { CPFUsageMeter } from "./cpf-usage-meter"
import { AccountBeneficiariesPanel } from "./account-beneficiaries-panel"
import { CopyCredentialsMenu } from "./copy-credentials-menu"
import { DeleteOverlay } from "./delete-overlay"
import { EditAccountPanel } from "./edit-account-panel"
import { AzulAccountControls } from "./azul-account-controls"
import { SmilesAccountControls } from "./smiles-account-controls"

interface AccountProgramBlockProps {
  account: Account
  people: PersonOption[]
  loyaltyPrograms: LoyaltyProgram[]
  canReconcile: boolean
}

// One loyalty account inside a holder's row: the credentials the operator signs in with,
// and every per-account action the old account card had. Merging works on the whole row
// (merge-account-holder-panel.tsx); the holder's aliases and administrator are shown once,
// under the holder.
export function AccountProgramBlock({
  account,
  people,
  loyaltyPrograms,
  canReconcile,
}: AccountProgramBlockProps) {
  const router = useRouter()
  const [isDeleteOpen, setIsDeleteOpen] = useState(false)
  // One panel at a time: they share the block's bottom edge, and two open at once would
  // both claim the rounded corner.
  const [openPanel, setOpenPanel] = useState<"none" | "edit" | "beneficiaries">(
    "none",
  )
  const [isDeleted, setIsDeleted] = useState(false)
  const [showPassword, setShowPassword] = useState(false)

  const isEditOpen = openPanel === "edit"
  const isBeneficiariesOpen = openPanel === "beneficiaries"

  function handleFadeEnd() {
    if (isDeleted) router.refresh()
  }

  const deleteLabel = `${account.loyalty_program.name}${account.login ? ` (${account.login})` : ""}`

  return (
    <div className="flex flex-col">
      <div
        className={cn(
          "relative overflow-hidden border border-slate-200 bg-white transition-opacity duration-300",
          openPanel === "none" ? "rounded-md" : "rounded-t-md",
          isDeleted ? "opacity-0" : "opacity-100",
        )}
        onTransitionEnd={handleFadeEnd}
      >
        <div className="flex items-start justify-between gap-3 px-4 py-3">
          <div className="flex min-w-0 flex-col gap-2">
            <div className="flex flex-wrap items-center gap-2">
              <LoyaltyProgramBadge loyaltyProgram={account.loyalty_program} />
              <AccountCategoryBadge category={account.category} />
              {account.waives_redemption_fee && <RedemptionFeeWaiverBadge />}
              {account.cpf_usage && <CPFUsageMeter usage={account.cpf_usage} />}
            </div>
            <dl className="grid grid-cols-[auto_1fr] gap-x-3 gap-y-1 text-sm">
              <dt className="text-slate-500">Login</dt>
              <dd className="truncate font-medium text-slate-800">
                {account.login ?? <span className="text-slate-400">—</span>}
              </dd>
              <dt className="text-slate-500">Senha</dt>
              <dd className="flex items-center gap-2 font-medium text-slate-800">
                {account.password ? (
                  <>
                    <span className="truncate font-mono">
                      {showPassword ? account.password : "••••••••"}
                    </span>
                    <button
                      type="button"
                      className="text-slate-500 hover:text-slate-800"
                      title={showPassword ? "Ocultar senha" : "Mostrar senha"}
                      aria-label={
                        showPassword ? "Ocultar senha" : "Mostrar senha"
                      }
                      onClick={() => setShowPassword((prev) => !prev)}
                    >
                      {showPassword ? (
                        <EyeOff className="h-4 w-4" />
                      ) : (
                        <Eye className="h-4 w-4" />
                      )}
                    </button>
                  </>
                ) : (
                  <span className="text-slate-400">—</span>
                )}
              </dd>
            </dl>
          </div>
          <div className="flex shrink-0 gap-2">
            <CopyCredentialsMenu
              login={account.login}
              password={account.password}
            />
            <Button
              variant="outline"
              size="icon"
              title="Pessoas emitidas nesta conta"
              aria-label="Pessoas emitidas nesta conta"
              onClick={() =>
                setOpenPanel((prev) =>
                  prev === "beneficiaries" ? "none" : "beneficiaries",
                )
              }
              className={cn(
                "transition-colors duration-200",
                isBeneficiariesOpen
                  ? "border-violet-600 bg-violet-600 text-white hover:border-violet-700 hover:bg-violet-700"
                  : "border-slate-800 text-slate-800 hover:border-violet-400 hover:bg-violet-400 hover:text-white active:border-violet-800 active:bg-violet-800 active:text-white",
              )}
            >
              <Users className="h-4 w-4" />
            </Button>
            <Button
              variant="outline"
              size="icon"
              title="Excluir conta"
              aria-label="Excluir conta"
              onClick={() => setIsDeleteOpen(true)}
              className="border-slate-800 text-slate-800 transition-colors duration-200 hover:border-red-600 hover:bg-red-600 hover:text-white active:border-red-950 active:bg-red-950 active:text-white"
            >
              <Trash2 className="h-4 w-4" />
            </Button>
            <Button
              variant="outline"
              size="icon"
              title="Editar conta"
              aria-label="Editar conta"
              onClick={() =>
                setOpenPanel((prev) => (prev === "edit" ? "none" : "edit"))
              }
              className={cn(
                "transition-colors duration-200",
                isEditOpen
                  ? "border-cyan-600 bg-cyan-600 text-white hover:border-cyan-700 hover:bg-cyan-700"
                  : "border-slate-800 text-slate-800 hover:border-cyan-400 hover:bg-cyan-400 hover:text-white active:border-cyan-800 active:bg-cyan-800 active:text-white",
              )}
            >
              <Menu className="h-4 w-4" />
            </Button>
          </div>
        </div>

        {/* Each strip renders only for its own programme, so at most one appears. */}
        <AzulAccountControls account={account} />
        <SmilesAccountControls account={account} />

        <DeleteOverlay
          accountName={deleteLabel}
          accountId={account.id}
          isOpen={isDeleteOpen}
          onDismissed={() => setIsDeleteOpen(false)}
          onDeleted={() => setIsDeleted(true)}
        />
      </div>

      <AccountBeneficiariesPanel
        account={account}
        isOpen={isBeneficiariesOpen}
        canReconcile={canReconcile}
      />

      <EditAccountPanel
        account={account}
        people={people}
        loyaltyPrograms={loyaltyPrograms}
        isOpen={isEditOpen}
        onSaved={() => setOpenPanel("none")}
      />
    </div>
  )
}

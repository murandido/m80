"use client"

import { useState } from "react"
import { useRouter } from "next/navigation"
import { Menu, Trash2 } from "lucide-react"
import { Button } from "@/components/ui/button"
import { cn } from "@/lib/utils"
import { Account } from "@/schemas/account"
import { Person } from "@/schemas/person"
import { LoyaltyProgram } from "@/schemas/loyalty-program"
import { LoyaltyProgramBadge } from "./loyalty-program-badge"
import { CopyCredentialsMenu } from "./copy-credentials-menu"
import { DeleteOverlay } from "./delete-overlay"
import { EditAccountPanel } from "./edit-account-panel"

interface AccountCardProps {
  account: Account
  people: Person[]
  loyaltyPrograms: LoyaltyProgram[]
}

export function AccountCard({ account, people, loyaltyPrograms }: AccountCardProps) {
  const router = useRouter()
  const [isDeleteOpen, setIsDeleteOpen] = useState(false)
  const [isEditOpen, setIsEditOpen] = useState(false)
  const [isDeleted, setIsDeleted] = useState(false)

  function handleCardFadeEnd() {
    if (isDeleted) router.refresh()
  }

  const holder = people.find((p) => p.id === account.holder_id)
  const administrator = people.find((p) => p.id === account.administrator_id)

  // Same fallback order as accountLabel in account-combobox.tsx, so one account
  // does not read as two different things depending on the screen. The alias
  // matters: an account the bot created from a WhatsApp message has no login, and
  // showing "Conta sem titular" hides the one name anybody would recognise it by.
  const title =
    holder?.name ?? account.login ?? account.nicknames[0]?.nickname ?? "Conta sem titular"
  const subtitle = [account.login, administrator?.name && `Admin: ${administrator.name}`]
    .filter(Boolean)
    .join(" · ")

  return (
    <div className="flex flex-col">
      <div
        className={cn(
          "relative overflow-hidden border border-slate-200 transition-opacity duration-300",
          isEditOpen ? "rounded-t-md" : "rounded-md",
          isDeleted ? "opacity-0" : "opacity-100",
        )}
        onTransitionEnd={handleCardFadeEnd}
      >
        <div className="flex items-center justify-between px-4 py-3">
          <div className="flex flex-col gap-1">
            <LoyaltyProgramBadge loyaltyProgram={account.loyalty_program} />
            <span className="text-sm font-medium text-slate-800">{title}</span>
            {subtitle && (
              <span className="text-xs text-slate-500">{subtitle}</span>
            )}
          </div>
          <div className="flex gap-2">
            <CopyCredentialsMenu login={account.login} password={account.password} />
            <Button
              variant="outline"
              size="icon"
              onClick={() => setIsDeleteOpen(true)}
              className="border-slate-800 text-slate-800 transition-colors duration-200 hover:border-red-600 hover:bg-red-600 hover:text-white active:border-red-950 active:bg-red-950 active:text-white"
            >
              <Trash2 className="h-4 w-4" />
            </Button>
            <Button
              variant="outline"
              size="icon"
              onClick={() => setIsEditOpen((prev) => !prev)}
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

        <DeleteOverlay
          accountName={title}
          accountId={account.id}
          isOpen={isDeleteOpen}
          onDismissed={() => setIsDeleteOpen(false)}
          onDeleted={() => setIsDeleted(true)}
        />
      </div>

      <EditAccountPanel
        account={account}
        people={people}
        loyaltyPrograms={loyaltyPrograms}
        isOpen={isEditOpen}
        onSaved={() => setIsEditOpen(false)}
      />
    </div>
  )
}

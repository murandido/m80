"use client"

import { ReactNode } from "react"
import { Merge } from "lucide-react"
import { Button } from "@/components/ui/button"
import { Separator } from "@/components/ui/separator"
import { Spinner } from "@/components/ui/spinner"
import { cn } from "@/lib/utils"

interface MergePanelProps {
  isOpen: boolean
  // The record the operator clicked. It survives the merge, and saying so at the top is
  // the whole safety story: the two sides are not interchangeable and cannot be undone.
  keepLabel: string
  subtitle: string
  // The picker for the duplicate, plus whatever the preview has to say about it.
  children: ReactNode
  confirmLabel: string
  canConfirm: boolean
  isPending: boolean
  error: string | null
  onConfirm: () => void
}

// The slide-open panel both merges share, in the same shape as the edit panels below the
// cards. Only the picker and the preview differ between people and accounts, so those come
// in as children rather than being written twice.
export function MergePanel({
  isOpen,
  keepLabel,
  subtitle,
  children,
  confirmLabel,
  canConfirm,
  isPending,
  error,
  onConfirm,
}: MergePanelProps) {
  return (
    <div
      className={cn(
        "grid transition-all duration-300 ease-in-out",
        isOpen ? "grid-rows-[1fr]" : "grid-rows-[0fr]",
      )}
    >
      <div className="overflow-hidden">
        <div className="flex flex-col gap-4 rounded-b-md border border-t-0 border-slate-200 p-4">
          <div>
            <h2 className="text-sm font-semibold text-slate-800">
              Mesclar em “{keepLabel}”
            </h2>
            <p className="mt-0.5 text-xs text-slate-500">{subtitle}</p>
          </div>

          {children}

          <Separator />

          <p className="text-xs text-amber-700">
            Esta ação não tem desfazer: o cadastro escolhido é apagado de vez.
          </p>

          {error && <p className="text-destructive text-sm">{error}</p>}

          <Button
            type="button"
            onClick={onConfirm}
            disabled={!canConfirm || isPending}
            className="h-full bg-slate-800 hover:bg-slate-900"
          >
            <div className="flex w-full items-center justify-center gap-2 p-1 text-sm text-slate-50">
              {isPending ? <Spinner /> : <Merge className="h-4 w-4" />}
              {isPending ? "Mesclando..." : confirmLabel}
            </div>
          </Button>
        </div>
      </div>
    </div>
  )
}

// PreviewLines renders the counted facts a merge is about to move. An empty list means the
// duplicate carries nothing but itself, which is worth saying out loud.
export function PreviewLines({ lines }: { lines: string[] }) {
  return (
    <ul className="flex flex-col gap-1 rounded-md bg-slate-50 p-3">
      {lines.length === 0 ? (
        <li className="text-xs text-slate-500">
          Nada a mover — o cadastro escolhido só será apagado.
        </li>
      ) : (
        lines.map((line) => (
          <li key={line} className="text-xs text-slate-600">
            {line}
          </li>
        ))
      )}
    </ul>
  )
}

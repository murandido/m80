import { Label } from "@/components/ui/label"
import { Skeleton } from "@/components/ui/skeleton"
import { SalesClientsTableSkeleton } from "@/components/sales/sales-clients-table"

// The payments screen in its real shape - header, filter bar and the table with its rows -
// so a navigation to it does not swap a spinner for the screen and jump. The table
// skeleton comes from the table itself and the table keeps drawing it until its first page
// arrives, so the rows stay in place from the click to the data.
//
// The Clientes/Fornecedores toggle is left out: it is admin-only, and knowing the role
// here would cost a request before the skeleton could paint. An admin sees the toggle push
// the content down once; everyone else sees no movement.
//
// inventory/ and suppliers/ have their own loading.tsx so they do not inherit this one.
export default function SalesLoading() {
  return (
    <div className="p-4 sm:p-8" aria-busy="true">
      <div className="flex flex-col gap-4 sm:gap-6">
        <div className="flex flex-wrap items-center justify-between gap-3">
          <h1 className="text-lg font-semibold text-slate-800">
            Controle de pagamentos
          </h1>
          {/* Gerenciar clientes, Exportar, PDF do cliente and Como funciona. */}
          <div className="flex flex-wrap items-center gap-2">
            <Skeleton className="h-9 w-40" />
            <Skeleton className="h-9 w-28" />
            <Skeleton className="h-9 w-36" />
            <Skeleton className="h-9 w-36" />
          </div>
        </div>

        {/* SalesFilterBar's clients variant: agency, period, search and the clear button. */}
        <div className="flex flex-col gap-3">
          <div className="flex flex-wrap items-end gap-3">
            <div className="flex w-full flex-col gap-1 sm:w-52">
              <Label className="text-xs text-slate-500">Agência</Label>
              <Skeleton className="h-9 w-full" />
            </div>
            <div className="flex flex-col gap-1">
              <Label className="text-xs text-slate-500">Período</Label>
              <Skeleton className="h-9 w-44" />
            </div>
            <div className="flex w-full flex-col gap-1.5 sm:w-72">
              <Label className="text-muted-foreground text-xs">Buscar</Label>
              <Skeleton className="h-9 w-full" />
            </div>
            <Skeleton className="h-8 w-32" />
          </div>
        </div>

        <SalesClientsTableSkeleton />
      </div>
    </div>
  )
}

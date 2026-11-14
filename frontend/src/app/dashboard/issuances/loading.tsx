import { Label } from "@/components/ui/label"
import { Skeleton } from "@/components/ui/skeleton"
import layout from "@/components/issuances/board/board-layout.module.css"

// How many placeholder cards the board shows while it loads. The default view is "today,
// issued by me", which is a handful of cards on a normal day.
const PLACEHOLDER_CARD_COUNT = 6

// The board in its real shape - title, search, filters, actions and a few cards - laid out
// by the same grid as the page, so the screen does not jump when the issuances arrive.
//
// It lives here and not in (grid)/ on purpose: a loading.tsx wraps the page of its own
// segment but not that segment's layout, and (grid)/layout.tsx is where the six reference
// lists are fetched. Placed in (grid)/, the dashboard spinner would still cover that wait.
// The ticket screen under [id] has its own loading.tsx so it does not get this one.
export default function IssuancesLoading() {
  return (
    <div className="p-4 sm:p-8" aria-busy="true">
      <div className={layout.layout}>
        <h1 className={`${layout.title} text-lg font-semibold text-slate-800`}>
          Emissões
        </h1>

        {/* ListSearchBar: the label row above a 36px input. */}
        <div className={`${layout.search} flex flex-col gap-1.5`}>
          <Label className="text-muted-foreground text-xs">Buscar</Label>
          <Skeleton className="h-9 w-full" />
        </div>

        {/* BoardFilters: period and filters, 40px tall, 12rem wide on a desktop. */}
        <div className={layout.filters}>
          <div className="grid grid-cols-2 gap-3 lg:flex lg:items-end">
            <div className="flex min-w-0 flex-col gap-1.5">
              <Label className="hidden text-slate-600 lg:block">Período</Label>
              <Skeleton className="h-10 w-full bg-slate-200 lg:w-48" />
            </div>
            <div className="flex min-w-0 flex-col gap-1.5">
              <Label className="hidden text-slate-600 lg:block">Filtros</Label>
              <Skeleton className="h-10 w-full bg-slate-200 lg:w-48" />
            </div>
          </div>
        </div>

        {/* BoardActions: Atalhos, Exportar and Simular emissão. */}
        <div className={layout.actions}>
          <div className="grid grid-cols-3 gap-2 lg:flex">
            <Skeleton className="h-10 w-full lg:w-28" />
            <Skeleton className="h-10 w-full lg:w-32" />
            <Skeleton className="h-10 w-full lg:w-40" />
          </div>
        </div>

        <hr className={`${layout.separator} border-slate-200`} />

        <div className={`${layout.board} flex flex-col gap-3`}>
          {/* Same grid and card anatomy as IssuanceBoard: the number and the status dot on
              top, then the four lines - program, locator, agency, issuer. */}
          <div className="grid grid-cols-1 gap-3 sm:grid-cols-[repeat(auto-fill,minmax(11rem,1fr))] sm:gap-3">
            {Array.from({ length: PLACEHOLDER_CARD_COUNT }, (_, index) => (
              <div key={index} className="rounded-2xl p-2 sm:aspect-square">
                <div className="flex w-full flex-col gap-2 rounded-lg border border-slate-200 bg-white p-2.5 sm:h-full sm:justify-between sm:gap-1">
                  <div className="flex items-center justify-between border-b border-slate-200 pb-2">
                    <Skeleton className="h-6 w-6 rounded-full" />
                    <Skeleton className="h-6 w-6 rounded-full" />
                  </div>
                  {["w-24", "w-20", "w-28", "w-24"].map((width, line) => (
                    <span key={line} className="flex min-w-0 items-center gap-1.5">
                      <Skeleton className="h-6 w-6 shrink-0 rounded-full sm:h-5 sm:w-5" />
                      <Skeleton className={`h-3.5 ${width}`} />
                    </span>
                  ))}
                </div>
              </div>
            ))}
          </div>
        </div>
      </div>
    </div>
  )
}

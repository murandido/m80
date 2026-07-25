"use client"

import { useMemo, useOptimistic, useState, useTransition } from "react"
import { useRouter } from "next/navigation"
import { RotateCcw } from "lucide-react"
import { toast } from "sonner"
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table"
import { Switch } from "@/components/ui/switch"
import {
  HoverCard,
  HoverCardContent,
  HoverCardTrigger,
} from "@/components/ui/hover-card"
import { ClientCombobox } from "@/components/clients/client-combobox"
import { updateSaleAction } from "@/actions/sale"
import { createClientAction } from "@/actions/client"
import { formatCurrencyBRL } from "@/lib/currency"
import { cn } from "@/lib/utils"
import type { Client } from "@/schemas/client"
import type { Sale } from "@/schemas/sale"

interface SalesClientsTableProps {
  sales: Sale[]
  clients: Client[]
}

const COLUMN_COUNT = 10

function formatPoints(points: number): string {
  return points.toLocaleString("pt-BR")
}

function formatDateTime(iso: string): string {
  return new Date(iso).toLocaleDateString("pt-BR")
}

// A reopened sale is one whose issuance was cancelled after the fact: the money may
// already have been collected and now has to go back. That is a different problem from a
// sale that was simply never charged, so it gets its own marker. Deleted issuances have no
// sale at all, so they never reach this table.
function ReopenedBadge({ sale }: { sale: Sale }) {
  if (sale.cancelled_at === null) return null

  return (
    <HoverCard>
      <HoverCardTrigger asChild>
        <span
          className={cn(
            // w-fit: the cell stacks the badge under the locator in a flex column, which
            // would otherwise stretch the pill across the whole column width.
            "inline-flex w-fit cursor-default items-center gap-1 rounded-full px-2 py-0.5",
            "bg-red-100 text-xs font-medium text-red-800",
          )}
        >
          <RotateCcw className="h-3 w-3" />
          Reaberta (cancelada)
        </span>
      </HoverCardTrigger>
      <HoverCardContent className="w-64 text-sm">
        <p className="font-medium">Emissão cancelada em {formatDateTime(sale.cancelled_at)}</p>
        <p className="mt-1 text-muted-foreground">
          O pagamento foi revertido automaticamente. Se o cliente já havia pago, o valor
          precisa ser devolvido ou compensado.
        </p>
      </HoverCardContent>
    </HoverCard>
  )
}

export function SalesClientsTable({ sales, clients }: SalesClientsTableProps) {
  const router = useRouter()
  const [, startTransition] = useTransition()
  const [savingIds, setSavingIds] = useState<Set<string>>(new Set())
  // Clients created from a row's combobox. The refresh that follows brings them in the
  // `clients` prop, but the combobox has to be able to show the new name immediately.
  const [createdClients, setCreatedClients] = useState<Client[]>([])

  const clientOptions = useMemo(() => {
    const known = new Set(clients.map((c) => c.id))
    return [...clients, ...createdClients.filter((c) => !known.has(c.id))]
  }, [clients, createdClients])

  // React drops each patch when the transition that made it ends - which is after the
  // refresh below landed the real rows. Hand-rolled overrides would have to be cleaned up
  // by hand, and on a screen that now refreshes itself from the SSE stream a leftover one
  // would hide the change another operator just made to the same sale.
  const [optimisticSales, applyOptimistic] = useOptimistic(
    sales,
    (current: Sale[], update: { saleId: string; patch: Partial<Sale> }) =>
      current.map((sale) =>
        sale.id === update.saleId ? { ...sale, ...update.patch } : sale,
      ),
  )

  function markSaving(saleId: string, saving: boolean) {
    setSavingIds((prev) => {
      const next = new Set(prev)
      if (saving) next.add(saleId)
      else next.delete(saleId)
      return next
    })
  }

  function persist(
    sale: Sale,
    patch: Partial<Sale>,
    payload: Parameters<typeof updateSaleAction>[1],
    errorMessage: string,
  ) {
    markSaving(sale.id, true)

    startTransition(async () => {
      // Applied before the await so the row updates on the click instead of after the
      // round trip; a rejected update simply never reaches the refresh, and React puts
      // the server's value back when this transition ends.
      applyOptimistic({ saleId: sale.id, patch })

      const result = await updateSaleAction(sale.id, payload)
      markSaving(sale.id, false)

      if ("error" in result && result.error) {
        toast.error(result.error || errorMessage)
        return
      }

      router.refresh()
    })
  }

  function handleClientChange(sale: Sale, clientId: string) {
    const client = clientOptions.find((c) => c.id === clientId)
    persist(
      sale,
      { client_id: clientId || null, client_name: client?.name ?? null },
      { client_id: clientId || null },
      "Erro ao vincular cliente.",
    )
  }

  // Creating and linking are one gesture from the operator's point of view, so a failure
  // to create must not leave the row looking linked - the link is only attempted once the
  // client exists and has an id.
  function handleClientCreate(sale: Sale, name: string) {
    markSaving(sale.id, true)

    startTransition(async () => {
      const result = await createClientAction({ name })
      markSaving(sale.id, false)

      if ("error" in result) {
        toast.error(result.error)
        return
      }

      setCreatedClients((prev) => [...prev, result.client])
      persist(
        sale,
        { client_id: result.client.id, client_name: result.client.name },
        { client_id: result.client.id },
        "Cliente criado, mas não foi possível vinculá-lo à venda.",
      )
    })
  }

  function handlePaidChange(sale: Sale, paid: boolean) {
    persist(sale, { paid }, { paid }, "Erro ao atualizar o pagamento.")
  }

  return (
    <div className="max-h-[calc(100vh-18rem)] overflow-auto rounded-md border">
      <Table>
        <TableHeader className="sticky top-0 z-10 bg-slate-100">
          <TableRow>
            <TableHead>Emissão</TableHead>
            <TableHead>Data</TableHead>
            <TableHead>Passageiro</TableHead>
            <TableHead>Conta</TableHead>
            <TableHead className="text-right">Pontos</TableHead>
            <TableHead className="text-right">Vlr. milheiro</TableHead>
            <TableHead className="text-right">Taxas</TableHead>
            <TableHead className="text-right">Valor</TableHead>
            <TableHead className="w-56">Cliente</TableHead>
            <TableHead className="w-32 text-center">Pago</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {optimisticSales.length === 0 && (
            <TableRow>
              <TableCell
                colSpan={COLUMN_COUNT}
                className="py-10 text-center text-sm text-muted-foreground"
              >
                Nenhuma venda no período selecionado.
              </TableCell>
            </TableRow>
          )}
          {optimisticSales.map((sale) => {
            const isSaving = savingIds.has(sale.id)
            const isReopened = sale.cancelled_at !== null

            return (
              <TableRow
                key={sale.id}
                className={cn(isReopened && "bg-muted/40", isSaving && "opacity-60")}
              >
                <TableCell>
                  <div className="flex flex-col gap-1">
                    <span
                      className={cn(
                        "font-medium",
                        isReopened && "text-muted-foreground line-through",
                      )}
                    >
                      {sale.program_locator}
                    </span>
                    <ReopenedBadge sale={sale} />
                  </div>
                </TableCell>
                <TableCell className="whitespace-nowrap text-muted-foreground">
                  {formatDateTime(sale.issued_at)}
                </TableCell>
                <TableCell>{sale.main_passenger_name ?? "—"}</TableCell>
                <TableCell>
                  <span className="flex min-w-0 items-center gap-2">
                    <span
                      className="h-2.5 w-2.5 shrink-0 rounded-full border border-slate-300"
                      style={{ backgroundColor: sale.loyalty_program_color }}
                    />
                    <span className="flex min-w-0 flex-col">
                      <span className="truncate">{sale.account_label}</span>
                      <span className="truncate text-xs text-muted-foreground">
                        {sale.loyalty_program_name}
                      </span>
                    </span>
                  </span>
                </TableCell>
                <TableCell className="text-right tabular-nums">
                  {formatPoints(sale.points)}
                </TableCell>
                <TableCell className="text-right tabular-nums text-muted-foreground">
                  {formatCurrencyBRL(sale.value_of_the_thousand)}
                </TableCell>
                <TableCell className="text-right tabular-nums text-muted-foreground">
                  {/* An em dash, not R$ 0,00: no taxes recorded is a different fact from
                      taxes that came to zero, and the operator has to be able to tell. */}
                  {sale.taxes === null ? "—" : formatCurrencyBRL(sale.taxes)}
                </TableCell>
                <TableCell className="text-right tabular-nums">
                  {formatCurrencyBRL(sale.amount_cents)}
                </TableCell>
                <TableCell>
                  <ClientCombobox
                    clients={clientOptions}
                    value={sale.client_id ?? ""}
                    onChange={(clientId) => handleClientChange(sale, clientId)}
                    onCreate={(name) => handleClientCreate(sale, name)}
                    disabled={isSaving}
                  />
                </TableCell>
                <TableCell>
                  <div className="flex items-center justify-center gap-2">
                    <Switch
                      checked={sale.paid}
                      onCheckedChange={(paid) => handlePaidChange(sale, paid)}
                      disabled={isSaving}
                      aria-label={sale.paid ? "Marcar como em aberto" : "Marcar como pago"}
                    />
                    <span
                      className={cn(
                        "text-xs",
                        sale.paid ? "text-emerald-700" : "text-muted-foreground",
                      )}
                    >
                      {sale.paid ? "Pago" : "Em aberto"}
                    </span>
                  </div>
                </TableCell>
              </TableRow>
            )
          })}
        </TableBody>
      </Table>
    </div>
  )
}

"use client"

import { useMemo, useState, useTransition } from "react"
import { toast } from "sonner"
import { Layers, Trash2 } from "lucide-react"
import { Button } from "@/components/ui/button"
import { DateInput } from "@/components/ui/date-input"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table"
import { Badge } from "@/components/ui/badge"
import { formatCurrencyBRL, parseCurrencyBRLInput } from "@/lib/currency"
import { formatSignedPaddedPoints, parsePointsFromThousands } from "@/lib/points"
import {
  createPointBatchAction,
  deletePointBatchAction,
  settlePointBatchAction,
} from "@/actions/inventory"
import type { Agency } from "@/schemas/agency"
import type { PointBatch, PointBatchOrigin } from "@/schemas/inventory"

interface Props {
  accountId: string
  batches: PointBatch[]
  // The window the toolbar is showing. The list is already filtered by it on the server;
  // what the panel does with it is date a new batch inside what is on screen and say so
  // when the account has batches that the filter is hiding.
  startsOn: string
  endsOn: string
  origins: PointBatchOrigin[]
  agencies: Agency[]
}

// Typed in milheiros and shown in full, which is the grammar the issuances grid already
// uses: "632,7" goes in, 632.700 comes back. One rule for point counts across the panel.
function parsePoints(text: string): number | null {
  const points = parsePointsFromThousands(text)
  if (points === null || points <= 0) return null
  return points
}

// The rate is fractional on purpose - 1413,2638 centavos per thousand - so it is shown
// with four decimals rather than rounded to the centavo. Rounding it is exactly the error
// the batch costing exists to avoid, and a figure that reads 14,13 here while the totals
// are built on 14,132638 invites somebody to "correct" the difference.
function formatRate(centsPerThousand: number): string {
  return (centsPerThousand / 100).toLocaleString("pt-BR", {
    minimumFractionDigits: 2,
    maximumFractionDigits: 4,
  })
}

function formatDate(value: string): string {
  return value ? value.split("-").reverse().join("/") : ""
}

// A new batch defaults to today, unless today is outside the window on screen - recording
// one that vanishes the moment it is saved is how somebody records it twice.
function defaultDate(startsOn: string, endsOn: string): string {
  const today = new Date().toISOString().slice(0, 10)
  if (startsOn && today < startsOn) return startsOn
  if (endsOn && today > endsOn) return endsOn
  return today
}

export function InventoryBatchesPanel({
  accountId,
  batches,
  startsOn,
  endsOn,
  origins,
  agencies,
}: Props) {
  const [pending, startTransition] = useTransition()
  const [originId, setOriginId] = useState("")
  const [acquiredOn, setAcquiredOn] = useState(() => defaultDate(startsOn, endsOn))
  const [milheiros, setMilheiros] = useState("")
  const [cost, setCost] = useState("")
  const [fundingAgencyId, setFundingAgencyId] = useState("")
  const [opening, setOpening] = useState(false)

  // What the cost field currently resolves to, in centavos. Null while it is blank or
  // half-typed, which is also when there is nothing worth confirming.
  const typedCost = useMemo(
    () => (cost.trim() === "" ? null : parseCurrencyBRLInput(cost)),
    [cost],
  )

  function submit() {
    const points = parsePoints(milheiros)
    if (points === null) {
      toast.error("Informe a quantidade de pontos.")
      return
    }
    // Zero is a legitimate cost - a club subscription and flight accrual both put points in
    // for nothing - so an empty field reads as zero rather than as an error.
    const costCents = cost.trim() === "" ? 0 : parseCurrencyBRLInput(cost)
    if (costCents === null) {
      toast.error("Custo inválido.")
      return
    }
    if (!fundingAgencyId) {
      toast.error("Informe a agência que bancou o lote.")
      return
    }
    if (!originId && !opening) {
      toast.error("Informe a origem do lote.")
      return
    }

    startTransition(async () => {
      const result = await createPointBatchAction({
        accountId,
        originId: opening ? null : originId,
        acquiredOn,
        points,
        costCents,
        fundingAgencyId,
        opening,
      })
      if ("error" in result) {
        toast.error(result.error)
        return
      }
      toast.success("Lote lançado.")
      setMilheiros("")
      setCost("")
      setOriginId("")
      setOpening(false)
    })
  }

  function remove(batch: PointBatch) {
    startTransition(async () => {
      const result = await deletePointBatchAction(batch.id)
      if ("error" in result) {
        toast.error(result.error)
        return
      }
      toast.success("Lote excluído. As emissões voltam para a fila no próximo cálculo.")
    })
  }

  function toggleSettled(batch: PointBatch) {
    startTransition(async () => {
      const result = await settlePointBatchAction(batch.id, !batch.settled_at)
      if ("error" in result) {
        toast.error(result.error)
        return
      }
      toast.success(batch.settled_at ? "Acerto desfeito." : "Lote marcado como acertado.")
    })
  }

  return (
    <section className="overflow-hidden rounded-lg border border-slate-200 dark:border-slate-700">
      <header className="flex flex-wrap items-center gap-3 bg-[#2F5496] px-4 py-2.5 text-white">
        <Layers className="size-4" />
        <h2 className="font-semibold">Lotes</h2>
        <span className="text-xs text-white/70">
          Cada compra de pontos, com o custo que ela teve — {formatDate(startsOn)} a{" "}
          {formatDate(endsOn)}, do mais novo para o mais antigo.
        </span>
      </header>
      <div className="flex flex-col gap-6 p-4">
        <div className="grid grid-cols-1 gap-3 sm:grid-cols-2 lg:grid-cols-6">
          <div className="flex flex-col gap-1.5">
            <Label htmlFor="batch-origin">Origem</Label>
            <Select value={originId} onValueChange={setOriginId} disabled={opening}>
              <SelectTrigger id="batch-origin">
                <SelectValue placeholder={opening ? "Saldo de partida" : "Escolha"} />
              </SelectTrigger>
              <SelectContent>
                {origins.map((origin) => (
                  <SelectItem key={origin.id} value={origin.id}>
                    {origin.name}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>

          <div className="flex flex-col gap-1.5">
            <Label htmlFor="batch-date">Data</Label>
            <DateInput
              id="batch-date"
              value={acquiredOn}
              onChange={setAcquiredOn}
            />
          </div>

          <div className="flex flex-col gap-1.5">
            <Label htmlFor="batch-points">
              Qnt. de Pontos
              <span className="ml-1 font-normal text-slate-400">em milheiros</span>
            </Label>
            <Input
              id="batch-points"
              inputMode="decimal"
              placeholder="632,7"
              value={milheiros}
              onChange={(event) => setMilheiros(event.target.value)}
            />
          </div>

          <div className="flex flex-col gap-1.5">
            <Label htmlFor="batch-cost">Custo (R$)</Label>
            <Input
              id="batch-cost"
              inputMode="decimal"
              placeholder="8.941,72"
              value={cost}
              onChange={(event) => setCost(event.target.value)}
            />
            {/* A dot and a comma both separate the decimals, so "1.500" is a real and a
                half rather than fifteen hundred. Saying what the field resolves to is what
                keeps that from being a surprise at the end of the month. */}
            {typedCost !== null && (
              <p className="text-xs text-slate-500">= {formatCurrencyBRL(typedCost)}</p>
            )}
          </div>

          <div className="flex flex-col gap-1.5">
            <Label htmlFor="batch-agency">Bancado por</Label>
            <Select value={fundingAgencyId} onValueChange={setFundingAgencyId}>
              <SelectTrigger id="batch-agency">
                <SelectValue placeholder="Escolha" />
              </SelectTrigger>
              <SelectContent>
                {agencies.map((agency) => (
                  <SelectItem key={agency.id} value={agency.id}>
                    {agency.name}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>

          <div className="flex items-end gap-2">
            <Button onClick={submit} disabled={pending} className="w-full">
              Lançar
            </Button>
          </div>
        </div>

        <label className="flex w-fit items-center gap-2 text-sm">
          <input
            type="checkbox"
            checked={opening}
            onChange={(event) => setOpening(event.target.checked)}
            className="size-4"
          />
          <span>
            Saldo de partida
            <span className="text-muted-foreground">
              {" "}
              — o que a conta já tinha ao entrar no painel, sem origem
            </span>
          </span>
        </label>

        <div className="overflow-x-auto">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>Origem</TableHead>
                <TableHead>Data</TableHead>
                <TableHead className="text-right">Qnt. de Pontos</TableHead>
                <TableHead className="text-right">Custo</TableHead>
                <TableHead className="text-right">Custo do milheiro</TableHead>
                <TableHead>Bancado por</TableHead>
                <TableHead>Rateio</TableHead>
                <TableHead>Acertado</TableHead>
                <TableHead />
              </TableRow>
            </TableHeader>
            <TableBody>
              {batches.length === 0 ? (
                <TableRow>
                  <TableCell colSpan={9} className="text-muted-foreground text-center">
                    Nenhum lote entre {formatDate(startsOn)} e {formatDate(endsOn)}. Amplie o
                    período acima, lance a primeira compra, ou concilie o extrato da companhia
                    mais abaixo.
                  </TableCell>
                </TableRow>
              ) : (
                batches.map((batch) => (
                  <TableRow key={batch.id}>
                    <TableCell>
                      {batch.origin?.name ?? (
                        <Badge variant="secondary">Saldo de partida</Badge>
                      )}
                    </TableCell>
                    <TableCell className="tabular-nums">
                      {batch.acquired_on.split("-").reverse().join("/")}
                    </TableCell>
                    <TableCell className="text-right tabular-nums">
                      {formatSignedPaddedPoints(batch.points)}
                    </TableCell>
                    <TableCell className="text-right tabular-nums text-amber-800 dark:text-amber-300">
                      {formatCurrencyBRL(batch.cost_cents)}
                    </TableCell>
                    <TableCell className="text-right tabular-nums text-amber-800 dark:text-amber-300">
                      {formatRate(batch.cost_of_the_thousand)}
                    </TableCell>
                    <TableCell>{batch.funding_agency?.name ?? "—"}</TableCell>
                    <TableCell className="tabular-nums">{batch.profit_share_percent}%</TableCell>
                    <TableCell>
                      {/* State first, action second: the label says what it is, and
                          clicking flips it. A filled button read as "press me". */}
                      <button
                        type="button"
                        disabled={pending}
                        onClick={() => toggleSettled(batch)}
                        title={batch.settled_at ? "Desfazer o acerto" : "Marcar como acertado"}
                        className={
                          batch.settled_at
                            ? "rounded-full bg-emerald-100 px-2.5 py-0.5 text-xs font-medium text-emerald-800 hover:bg-emerald-200 dark:bg-emerald-950 dark:text-emerald-300"
                            : "rounded-full bg-slate-100 px-2.5 py-0.5 text-xs font-medium text-slate-600 hover:bg-slate-200 dark:bg-slate-800 dark:text-slate-300"
                        }
                      >
                        {batch.settled_at ? "Acertado" : "Em aberto"}
                      </button>
                    </TableCell>
                    <TableCell className="text-right">
                      <Button
                        variant="ghost"
                        size="icon"
                        disabled={pending}
                        onClick={() => remove(batch)}
                        aria-label="Excluir lote"
                      >
                        <Trash2 className="size-4" />
                      </Button>
                    </TableCell>
                  </TableRow>
                ))
              )}
            </TableBody>
          </Table>
        </div>
      </div>
    </section>
  )
}

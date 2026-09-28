import { Badge } from "@/components/ui/badge"
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table"
import { cn } from "@/lib/utils"
import type { AirlineLookupReport, LookupFailure } from "@/schemas/booking-lookup"

// Temporary, with the report it renders: see src/schemas/booking-lookup.ts.

const OUTCOME_LABELS: Record<string, string> = {
  found: "Achou",
  not_found: "Não achou",
  unavailable: "Fora do ar",
  refused: "Recusada",
}

const dateTime = new Intl.DateTimeFormat("pt-BR", {
  day: "2-digit",
  month: "2-digit",
  hour: "2-digit",
  minute: "2-digit",
  timeZone: "America/Sao_Paulo",
})

// The share of the syncs where the airline was asked that it answered itself - the number
// that has to reach 100% before Traveos can be turned off for it.
function directShare(line: AirlineLookupReport): string {
  const asked = line.answered_by_direct + line.fell_back_to_traveos
  if (asked === 0) return "—"
  return `${Math.round((line.answered_by_direct / asked) * 100)}%`
}

function seconds(ms: number): string {
  if (ms === 0) return "—"
  return `${(ms / 1000).toLocaleString("pt-BR", { maximumFractionDigits: 1 })} s`
}

function ConnectionBadge({ line }: { line: AirlineLookupReport }) {
  if (!line.direct_provider) {
    return <Badge variant="outline">Sem integração</Badge>
  }
  if (line.direct_booking_enabled) {
    return <Badge className="bg-emerald-600 text-white">Ligada</Badge>
  }
  return <Badge variant="secondary">Desligada</Badge>
}

function Failures({ failures }: { failures: LookupFailure[] }) {
  if (failures.length === 0) return null
  return (
    <details className="mt-1">
      <summary className="cursor-pointer text-xs text-slate-500">
        {failures.length === 1 ? "1 falha recente" : `${failures.length} falhas recentes`}
      </summary>
      <ul className="mt-2 flex flex-col gap-1.5">
        {failures.map((failure, index) => (
          <li key={`${failure.issuance_id}-${index}`} className="text-xs text-slate-600">
            <span className="font-mono font-semibold text-slate-700">{failure.locator}</span>
            {" · "}
            {dateTime.format(new Date(failure.created_at))}
            {" · "}
            <span
              className={cn(
                failure.outcome === "unavailable" ? "text-red-600" : "text-amber-700",
              )}
            >
              {OUTCOME_LABELS[failure.outcome] ?? failure.outcome}
            </span>
            {" · "}
            {failure.answered_by
              ? `respondida pela ${failure.answered_by === "traveos" ? "Traveos" : failure.answered_by}`
              : "ninguém achou"}
            <p className="break-all text-slate-400">{failure.error}</p>
          </li>
        ))}
      </ul>
    </details>
  )
}

export function DirectLookupsTable({ airlines }: { airlines: AirlineLookupReport[] }) {
  return (
    <Table>
      <TableHeader>
        <TableRow>
          <TableHead>Companhia</TableHead>
          <TableHead>Conexão direta</TableHead>
          <TableHead className="text-right">Consultas</TableHead>
          <TableHead className="text-right">Direta respondeu</TableHead>
          <TableHead className="text-right">Caiu para a Traveos</TableHead>
          <TableHead className="text-right">Ninguém achou</TableHead>
          <TableHead className="text-right">Direta: não achou / fora do ar</TableHead>
          <TableHead className="text-right">Tempo médio</TableHead>
        </TableRow>
      </TableHeader>
      <TableBody>
        {airlines.map((line) => (
          <TableRow key={line.airline_id} className="align-top">
            <TableCell>
              <div className="font-medium text-slate-800">{line.name}</div>
              <div className="text-xs text-slate-500">{line.code}</div>
              <Failures failures={line.recent_failures} />
            </TableCell>
            <TableCell>
              <ConnectionBadge line={line} />
            </TableCell>
            <TableCell className="text-right tabular-nums">{line.syncs}</TableCell>
            <TableCell className="text-right tabular-nums">
              {line.answered_by_direct}
              <span className="ml-1 text-xs text-slate-500">({directShare(line)})</span>
            </TableCell>
            <TableCell
              className={cn(
                "text-right tabular-nums",
                line.fell_back_to_traveos > 0 && "font-semibold text-amber-700",
              )}
            >
              {line.fell_back_to_traveos}
            </TableCell>
            <TableCell
              className={cn(
                "text-right tabular-nums",
                line.unanswered > 0 && "font-semibold text-red-600",
              )}
            >
              {line.unanswered}
            </TableCell>
            <TableCell className="text-right tabular-nums">
              {line.direct_not_found} / {line.direct_unavailable}
            </TableCell>
            <TableCell className="text-right tabular-nums">
              {seconds(line.direct_average_ms)}
            </TableCell>
          </TableRow>
        ))}
      </TableBody>
    </Table>
  )
}

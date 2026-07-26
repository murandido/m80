import {
  Table,
  TableBody,
  TableCell,
  TableFooter,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table"
import { formatPoints } from "@/lib/format"
import { cn } from "@/lib/utils"
import type { SupplierBalance } from "@/schemas/sale"

interface SalesSuppliersTableProps {
  balances: SupplierBalance[]
}

const COLUMN_COUNT = 6

export function SalesSuppliersTable({ balances }: SalesSuppliersTableProps) {
  // Summed in the browser rather than asked of the API: the table does not paginate, so
  // these are exactly the rows on screen - which is what "total do que está aparecendo"
  // means. A server-side total could disagree with the visible rows and nobody would know
  // which one to trust.
  const totals = balances.reduce(
    (acc, balance) => ({
      issued: acc.issued + balance.issued_points,
      refunded: acc.refunded + balance.refunded_points,
      net: acc.net + balance.net_points,
    }),
    { issued: 0, refunded: 0, net: 0 },
  )

  return (
    <div className="max-h-[calc(100vh-22rem)] overflow-auto rounded-md border">
      <Table>
        <TableHeader className="sticky top-0 z-10 bg-slate-100">
          <TableRow>
            <TableHead>Conta</TableHead>
            <TableHead>Programa</TableHead>
            <TableHead>Administrador</TableHead>
            <TableHead className="text-right">Milhas emitidas</TableHead>
            <TableHead className="text-right">Milhas devolvidas</TableHead>
            <TableHead className="text-right">Milhas líquidas</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {balances.length === 0 && (
            <TableRow>
              <TableCell
                colSpan={COLUMN_COUNT}
                className="py-10 text-center text-sm text-muted-foreground"
              >
                Nenhuma emissão no período selecionado.
              </TableCell>
            </TableRow>
          )}
          {balances.map((balance) => (
            <TableRow key={balance.account_id}>
              <TableCell className="font-medium">{balance.account_label}</TableCell>
              <TableCell>
                <span className="flex min-w-0 items-center gap-2">
                  <span
                    className="h-2.5 w-2.5 shrink-0 rounded-full border border-slate-300"
                    style={{ backgroundColor: balance.loyalty_program_color }}
                  />
                  <span className="truncate">{balance.loyalty_program_name}</span>
                </span>
              </TableCell>
              <TableCell>{balance.administrator_name ?? "—"}</TableCell>
              <TableCell className="text-right tabular-nums">
                {formatPoints(balance.issued_points)}
              </TableCell>
              <TableCell className="text-right tabular-nums text-muted-foreground">
                {balance.refunded_points > 0
                  ? `-${formatPoints(balance.refunded_points)}`
                  : "—"}
              </TableCell>
              <TableCell
                className={cn(
                  "text-right font-medium tabular-nums",
                  balance.net_points < 0 && "text-red-700",
                )}
              >
                {formatPoints(balance.net_points)}
              </TableCell>
            </TableRow>
          ))}
        </TableBody>
        {balances.length > 0 && (
          <TableFooter className="sticky bottom-0 bg-slate-100">
            <TableRow>
              <TableCell colSpan={3} className="font-medium">
                Total ({balances.length} {balances.length === 1 ? "conta" : "contas"})
              </TableCell>
              <TableCell className="text-right font-medium tabular-nums">
                {formatPoints(totals.issued)}
              </TableCell>
              <TableCell className="text-right font-medium tabular-nums text-muted-foreground">
                {totals.refunded > 0 ? `-${formatPoints(totals.refunded)}` : "—"}
              </TableCell>
              <TableCell
                className={cn(
                  "text-right font-semibold tabular-nums",
                  totals.net < 0 && "text-red-700",
                )}
              >
                {formatPoints(totals.net)}
              </TableCell>
            </TableRow>
          </TableFooter>
        )}
      </Table>
    </div>
  )
}

import type { SupplierBalance, SupplierIssuance } from "@/schemas/sale"

// What the agency pays for one statement line, in centavos, or null while the cost has not
// been agreed. Null is not zero: a missing cost is what stops the payment summary from
// being generated, and rendering it as R$ 0,00 would hide exactly that.
//
// The rounding is the project's own - points/1000 * rate, rounded half up - and matches
// supplierLineAmount in backend/internal/api/v1/handlers/sales_report.go as well as
// issuanceTotalCents in components/issuances/emissao-message.ts. All three have to agree to
// the centavo, because the operator reads one, the supplier reads another, and the bot
// checksums the third.
//
// cost_cents overrides all of that on an account that owns its points, and has to: there
// the cost is the sum of the batch slices that paid for the booking, and the rate beside it
// is that sum rounded to a whole centavo per thousand for display. A booking split across
// two batches - 360 milheiros costing R$ 5.013,53 across rates of 14,1326 and 13,58 -
// averages out to 13,93, and multiplying that back gives R$ 5.014,80. Off by R$ 1,27 on one
// line, in a direction no line can explain.
//
// A refund line also carries the supplier's fee for the CPFs the cancellation burned, which
// is paid on top of the credit. The PDF prints it as a line of its own under the credit; the
// totals agree because both add it exactly once.
export function supplierLineAmount(
  line: Pick<
    SupplierIssuance,
    "points" | "taxes" | "cost_of_the_thousand" | "cost_cents" | "taxes_included_in_cost"
  > &
    Partial<Pick<SupplierIssuance, "cancelled_cpf_fee">>,
): number | null {
  const taxes = line.taxes_included_in_cost ? (line.taxes ?? 0) : 0
  const extras = taxes + (line.cancelled_cpf_fee ?? 0)

  if (line.cost_cents !== null && line.cost_cents !== undefined) {
    return line.cost_cents + extras
  }
  if (line.cost_of_the_thousand === null) return null

  return roundHalfUp(line.points * line.cost_of_the_thousand) + extras
}

// Rounds half away from zero, so a credit rounds the same distance as the booking it
// reverses and the pair cancels out exactly. Math.round() rounds -0.5 up to -0, which would
// leave a centavo behind on every cancelled-and-rebooked issuance.
function roundHalfUp(product: number): number {
  return product < 0 ? -Math.round((-product) / 1000) : Math.round(product / 1000)
}

export interface SupplierTotals {
  points: number
  taxes: number
  amount: number
  // cpfFees totals the cancelled-CPF fees, already inside amount, for the column that shows
  // them on their own.
  cpfFees: number
  // missingCost counts the lines whose cost has not been agreed. It is what disables the
  // PDF button and says how many are left, before the backend refuses with a 422.
  missingCost: number
}

export function sumSupplierLines(lines: SupplierIssuance[]): SupplierTotals {
  return lines.reduce<SupplierTotals>(
    (totals, line) => {
      const amount = supplierLineAmount(line)
      return {
        points: totals.points + line.points,
        // Only the taxes actually being reimbursed are totalled, so the column adds up to
        // the part of the amount it is responsible for.
        taxes: totals.taxes + (line.taxes_included_in_cost ? (line.taxes ?? 0) : 0),
        amount: totals.amount + (amount ?? 0),
        cpfFees: totals.cpfFees + (amount === null ? 0 : (line.cancelled_cpf_fee ?? 0)),
        missingCost: totals.missingCost + (amount === null ? 1 : 0),
      }
    },
    { points: 0, taxes: 0, amount: 0, cpfFees: 0, missingCost: 0 },
  )
}

export function sumSupplierBalances(
  balances: SupplierBalance[],
): SupplierTotals & { issued: number; refunded: number; net: number } {
  return balances.reduce(
    (totals, balance) => {
      const lines = sumSupplierLines(balance.issuances)
      return {
        issued: totals.issued + balance.issued_points,
        refunded: totals.refunded + balance.refunded_points,
        net: totals.net + balance.net_points,
        points: totals.points + lines.points,
        taxes: totals.taxes + lines.taxes,
        amount: totals.amount + lines.amount,
        cpfFees: totals.cpfFees + lines.cpfFees,
        missingCost: totals.missingCost + lines.missingCost,
      }
    },
    { issued: 0, refunded: 0, net: 0, points: 0, taxes: 0, amount: 0, cpfFees: 0, missingCost: 0 },
  )
}

"use server"

import { cookies } from "next/headers"
import { revalidatePath } from "next/cache"
import { env } from "@/lib/env"

// The backend validates issuance_ids with max=1000, so a request is split before it is
// sent rather than refused after. The bulk buttons pass everything on screen, and a busy
// month goes past a thousand lines - which used to mean the button failed exactly when it
// was most useful.
const MAX_IDS_PER_REQUEST = 1000

interface SupplierTerms {
  costOfTheThousand?: number | null
  taxesIncludedInCost?: boolean
  reviewed?: boolean
  // Centavos; null clears it. Only accepted on cancelled issuances.
  cancelledCpfFee?: number | null
}

function buildBody(issuanceIds: string[], terms: SupplierTerms) {
  // Keys are added only when the caller asked for them: the backend tells an absent
  // cost_of_the_thousand (leave the rate) from an explicit null (clear it), and
  // JSON.stringify drops undefined, which is exactly the distinction we need.
  return JSON.stringify({
    issuance_ids: issuanceIds,
    ...("costOfTheThousand" in terms ? { cost_of_the_thousand: terms.costOfTheThousand } : {}),
    ...(terms.taxesIncludedInCost !== undefined
      ? { taxes_included_in_cost: terms.taxesIncludedInCost }
      : {}),
    ...(terms.reviewed !== undefined ? { reviewed: terms.reviewed } : {}),
    // Same rule as the rate: absent leaves the fee alone, null clears it.
    ...("cancelledCpfFee" in terms ? { cancelled_cpf_fee: terms.cancelledCpfFee } : {}),
  })
}

// Sets the payment terms on a set of issuances - the cost per thousand miles in centavos
// (null clears it back to "not agreed yet"), whether the taxes are reimbursed, and whether
// the line has been checked while reconciling.
//
// An omitted term is left alone, which is why they are optional fields rather than an
// object with defaults - sending the taxes alone must not wipe the rates.
//
// The caller passes the ids it has on screen rather than the filters that produced them, so
// the write can only ever land on the rows the operator saw and confirmed. The backend
// refuses an empty list for the same reason.
export async function updateSupplierIssuancesAction(
  issuanceIds: string[],
  terms: SupplierTerms,
) {
  if (issuanceIds.length === 0) {
    return { error: "Nenhuma emissão selecionada." }
  }

  const cookieStore = await cookies()
  const token = cookieStore.get("auth_token")?.value

  const batches: string[][] = []
  for (let index = 0; index < issuanceIds.length; index += MAX_IDS_PER_REQUEST) {
    batches.push(issuanceIds.slice(index, index + MAX_IDS_PER_REQUEST))
  }

  let updated = 0
  try {
    // Sequential rather than parallel: these are UPDATEs on the same table, and the point
    // of the batching is to stay within a limit the server set, not to go faster.
    for (const batch of batches) {
      const res = await fetch(`${env.BACKEND_URL}/api/v1/sales/suppliers/issuances`, {
        method: "POST",
        headers: {
          "Content-Type": "application/json",
          Authorization: `Bearer ${token}`,
        },
        body: buildBody(batch, terms),
      })

      const json = await res.json().catch(() => null)
      if (!res.ok) {
        // Earlier batches already landed, so the count goes back with the error. Reporting
        // a clean failure after writing nine hundred rows would send the operator looking
        // for a change that is already there.
        const message = json?.message || "Erro ao gravar os termos de pagamento."
        revalidatePath("/dashboard/sales/suppliers")
        return updated > 0
          ? { error: `${message} ${updated} emissões já haviam sido gravadas.` }
          : { error: message }
      }
      updated += Number(json?.data?.updated ?? 0)
    }

    revalidatePath("/dashboard/sales/suppliers")
    return { success: true as const, updated }
  } catch {
    return { error: "Erro de conexão com o servidor." }
  }
}

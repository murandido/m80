"use server"

import { cookies } from "next/headers"
import { revalidatePath } from "next/cache"
import { env } from "@/lib/env"

// Sets the payment terms on a set of issuances: the cost per thousand miles in centavos
// (null clears it back to "not agreed yet") and/or whether the taxes are reimbursed.
//
// An omitted term is left alone, which is why the two are separate optional arguments
// rather than one object with defaults - sending the taxes alone must not wipe the rates.
//
// The caller passes the ids it has on screen rather than the filters that produced them, so
// the write can only ever land on the rows the operator saw and confirmed. The backend
// refuses an empty list for the same reason.
export async function updateSupplierIssuancesAction(
  issuanceIds: string[],
  terms: { costOfTheThousand?: number | null; taxesIncludedInCost?: boolean },
) {
  if (issuanceIds.length === 0) {
    return { error: "Nenhuma emissão selecionada." }
  }

  const cookieStore = await cookies()
  const token = cookieStore.get("auth_token")?.value

  try {
    const res = await fetch(`${env.BACKEND_URL}/api/v1/sales/suppliers/issuances`, {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
        Authorization: `Bearer ${token}`,
      },
      // Keys are added only when the caller asked for them: the backend tells an absent
      // cost_of_the_thousand (leave the rate) from an explicit null (clear it), and
      // JSON.stringify drops undefined, which is exactly the distinction we need.
      body: JSON.stringify({
        issuance_ids: issuanceIds,
        ...("costOfTheThousand" in terms
          ? { cost_of_the_thousand: terms.costOfTheThousand }
          : {}),
        ...(terms.taxesIncludedInCost !== undefined
          ? { taxes_included_in_cost: terms.taxesIncludedInCost }
          : {}),
      }),
    })

    const json = await res.json().catch(() => null)
    if (!res.ok) {
      return { error: json?.message || "Erro ao gravar os termos de pagamento." }
    }

    revalidatePath("/dashboard/sales/suppliers")
    return { success: true as const, updated: Number(json?.data?.updated ?? 0) }
  } catch {
    return { error: "Erro de conexão com o servidor." }
  }
}

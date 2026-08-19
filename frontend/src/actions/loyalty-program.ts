"use server"

import { cookies } from "next/headers"
import { env } from "@/lib/env"
import { parseCurrencyBRLInput } from "@/lib/currency"
import { createLoyaltyProgramSchema, updateLoyaltyProgramSchema } from "@/schemas/loyalty-program"
import { z } from "zod"

// The form uses "" for "this program has no researched cap", which is not something the
// API can read: cpf_limit is a number there, and the two enums reject an empty string. So
// the empty ones become explicit nulls, which is how the API stores "no cap".
function withNullableCaps<T extends Record<string, unknown>>(data: T) {
  const normalized: Record<string, unknown> = { ...data }
  for (const key of ["cpf_limit", "cpf_limit_window", "cpf_limit_mode"]) {
    if (normalized[key] === "" || normalized[key] === undefined) normalized[key] = null
  }
  // The limit travels as text through the form; the API takes a number.
  if (typeof normalized.cpf_limit === "string") {
    normalized.cpf_limit = Number(normalized.cpf_limit)
  }

  // The redemption fee's three parameters follow the same "" = "not configured" rule. The
  // two amounts are typed in reais and stored in centavos, so they are renamed on the way
  // out as well as converted - the form field is not the API field.
  normalized.redemption_fee_domestic_cents = toCents(normalized.redemption_fee_domestic)
  normalized.redemption_fee_international_cents = toCents(
    normalized.redemption_fee_international,
  )
  delete normalized.redemption_fee_domestic
  delete normalized.redemption_fee_international
  normalized.redemption_fee_advance_days =
    normalized.redemption_fee_advance_days === "" ||
    normalized.redemption_fee_advance_days === undefined
      ? null
      : Number(normalized.redemption_fee_advance_days)

  return normalized
}

// Reais as typed into the form, into integer centavos. Null for an empty field, which is
// what tells the API this program has no fee at all.
function toCents(value: unknown): number | null {
  if (typeof value !== "string" || value.trim() === "") return null
  return parseCurrencyBRLInput(value)
}

export async function createLoyaltyProgramAction(
  data: z.infer<typeof createLoyaltyProgramSchema.request>,
) {
  const cookieStore = await cookies()
  const token = cookieStore.get("auth_token")?.value

  try {
    const res = await fetch(`${env.BACKEND_URL}/api/v1/loyalty-programs`, {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
        Authorization: `Bearer ${token}`,
      },
      body: JSON.stringify(withNullableCaps(data)),
    })

    const json = await res.json()
    const parsed = createLoyaltyProgramSchema.response.safeParse(json)

    if (!res.ok) {
      return { error: parsed.data?.message || "Erro ao cadastrar programa de fidelidade." }
    }

    return { success: true }
  } catch {
    return { error: "Erro de conexão com o servidor." }
  }
}

export async function updateLoyaltyProgramAction(
  loyaltyProgramId: string,
  data: z.infer<typeof updateLoyaltyProgramSchema.request>,
) {
  const cookieStore = await cookies()
  const token = cookieStore.get("auth_token")?.value

  try {
    const res = await fetch(
      `${env.BACKEND_URL}/api/v1/loyalty-programs/${loyaltyProgramId}`,
      {
        method: "PUT",
        headers: {
          "Content-Type": "application/json",
          Authorization: `Bearer ${token}`,
        },
        body: JSON.stringify(withNullableCaps(data)),
      },
    )

    const json = await res.json()
    const parsed = updateLoyaltyProgramSchema.response.safeParse(json)

    if (!res.ok) {
      return { error: parsed.data?.message || "Erro ao atualizar programa de fidelidade." }
    }

    return { success: true }
  } catch {
    return { error: "Erro de conexão com o servidor." }
  }
}

export async function deleteLoyaltyProgramAction(loyaltyProgramId: string) {
  const cookieStore = await cookies()
  const token = cookieStore.get("auth_token")?.value

  try {
    const res = await fetch(
      `${env.BACKEND_URL}/api/v1/loyalty-programs/${loyaltyProgramId}`,
      {
        method: "DELETE",
        headers: { Authorization: `Bearer ${token}` },
      },
    )

    if (!res.ok) {
      return { error: "Erro ao deletar programa de fidelidade." }
    }

    return { success: true }
  } catch {
    return { error: "Erro de conexão com o servidor." }
  }
}

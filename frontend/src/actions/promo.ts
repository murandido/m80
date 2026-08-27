"use server"

import { cookies } from "next/headers"
import { env } from "@/lib/env"
import {
  createLoyaltyDocumentSchema,
  createPromoRecipeSchema,
  testPromoRecipeSchema,
  toPromoRecipePayload,
  updateLoyaltyDocumentSchema,
  updatePromoRecipeSchema,
} from "@/schemas/promo"
import { z } from "zod"

async function authHeaders() {
  const cookieStore = await cookies()
  const token = cookieStore.get("auth_token")?.value
  return {
    "Content-Type": "application/json",
    Authorization: `Bearer ${token}`,
  }
}

export async function createPromoRecipeAction(
  data: z.infer<typeof createPromoRecipeSchema.request>,
) {
  try {
    const res = await fetch(`${env.BACKEND_URL}/api/v1/promo-recipes`, {
      method: "POST",
      headers: await authHeaders(),
      body: JSON.stringify(toPromoRecipePayload(data)),
    })

    const json = await res.json()
    const parsed = createPromoRecipeSchema.response.safeParse(json)

    if (!res.ok) {
      return { error: parsed.data?.message || "Erro ao cadastrar receita." }
    }

    return { success: true }
  } catch {
    return { error: "Erro de conexão com o servidor." }
  }
}

export async function updatePromoRecipeAction(
  recipeId: string,
  data: z.infer<typeof updatePromoRecipeSchema.request>,
) {
  try {
    const res = await fetch(`${env.BACKEND_URL}/api/v1/promo-recipes/${recipeId}`, {
      method: "PUT",
      headers: await authHeaders(),
      body: JSON.stringify(toPromoRecipePayload(data)),
    })

    const json = await res.json()
    const parsed = updatePromoRecipeSchema.response.safeParse(json)

    if (!res.ok) {
      return { error: parsed.data?.message || "Erro ao atualizar receita." }
    }

    return { success: true }
  } catch {
    return { error: "Erro de conexão com o servidor." }
  }
}

export async function deletePromoRecipeAction(recipeId: string) {
  try {
    const res = await fetch(`${env.BACKEND_URL}/api/v1/promo-recipes/${recipeId}`, {
      method: "DELETE",
      headers: await authHeaders(),
    })

    if (!res.ok) {
      return { error: "Erro ao excluir receita." }
    }

    return { success: true }
  } catch {
    return { error: "Erro de conexão com o servidor." }
  }
}

// testPromoRecipeAction is the dry run: it opens the page, tries to find the form and
// takes a screenshot, without submitting anything. The recipe travels in the body
// rather than by id, so an unsaved one can be tried - which is what makes a campaign
// fixable inside the two days it runs for.
//
// A page where nothing was found is a *successful* test with a negative answer: the
// screenshot and the reason are exactly what the author needs, so it comes back as
// data rather than as an error.
export async function testPromoRecipeAction(
  data: z.infer<typeof testPromoRecipeSchema.request>,
) {
  try {
    const res = await fetch(`${env.BACKEND_URL}/api/v1/promo-recipes/test`, {
      method: "POST",
      headers: await authHeaders(),
      body: JSON.stringify({
        url: data.url,
        cpf_selector: data.cpf_selector.trim() || null,
        checkbox_selector: data.checkbox_selector.trim() || null,
        submit_selector: data.submit_selector.trim() || null,
        success_pattern: data.success_pattern.trim() || null,
        failure_pattern: data.failure_pattern.trim() || null,
        wait_seconds:
          data.wait_seconds.trim() === "" ? null : Number(data.wait_seconds.trim()),
      }),
    })

    const json = await res.json()
    const parsed = testPromoRecipeSchema.response.safeParse(json)

    if (!res.ok) {
      return { error: parsed.data?.message || "Erro ao testar a receita." }
    }
    if (!parsed.success || !parsed.data.data) {
      return { error: "Resposta inesperada do servidor." }
    }

    return { result: parsed.data.data, message: parsed.data.message }
  } catch {
    // A browser run takes time, so a timeout here is the likely failure and worth
    // naming: "erro de conexão" alone would send someone looking at the wrong thing.
    return { error: "Não consegui falar com o serviço de promoções (ele pode estar fora do ar)." }
  }
}

export async function createLoyaltyDocumentAction(
  data: z.infer<typeof createLoyaltyDocumentSchema.request>,
) {
  try {
    const res = await fetch(`${env.BACKEND_URL}/api/v1/loyalty-documents`, {
      method: "POST",
      headers: await authHeaders(),
      body: JSON.stringify(data),
    })

    const json = await res.json()
    const parsed = createLoyaltyDocumentSchema.response.safeParse(json)

    if (!res.ok) {
      return { error: parsed.data?.message || "Erro ao cadastrar CPF." }
    }

    return { success: true }
  } catch {
    return { error: "Erro de conexão com o servidor." }
  }
}

export async function updateLoyaltyDocumentAction(
  documentId: string,
  data: z.infer<typeof updateLoyaltyDocumentSchema.request>,
) {
  try {
    const res = await fetch(`${env.BACKEND_URL}/api/v1/loyalty-documents/${documentId}`, {
      method: "PUT",
      headers: await authHeaders(),
      body: JSON.stringify(data),
    })

    const json = await res.json()
    const parsed = updateLoyaltyDocumentSchema.response.safeParse(json)

    if (!res.ok) {
      return { error: parsed.data?.message || "Erro ao atualizar CPF." }
    }

    return { success: true }
  } catch {
    return { error: "Erro de conexão com o servidor." }
  }
}

export async function deleteLoyaltyDocumentAction(documentId: string) {
  try {
    const res = await fetch(`${env.BACKEND_URL}/api/v1/loyalty-documents/${documentId}`, {
      method: "DELETE",
      headers: await authHeaders(),
    })

    const json = await res.json().catch(() => null)

    if (!res.ok) {
      // The API answers 422 with a sentence telling the operator to deactivate
      // instead, because a document with history cannot be removed without taking
      // the history with it. Passing that through matters more than a generic line.
      return { error: json?.message || "Erro ao excluir CPF." }
    }

    return { success: true }
  } catch {
    return { error: "Erro de conexão com o servidor." }
  }
}

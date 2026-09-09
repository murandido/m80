"use server"

import { cookies } from "next/headers"
import { revalidatePath } from "next/cache"
import { env } from "@/lib/env"
import {
  statementCommitResponseSchema,
  statementReconciliationResponseSchema,
  type StatementReconciliation,
} from "@/schemas/inventory"

// Server actions for the statement reconciliation. They forward the operator's file to the
// Go API with the JWT attached, which is the panel's standard mutation path.
//
// The file is uploaded again on every call rather than parked server-side. It is a few
// kilobytes, and nothing about a statement is stored - the whole feature leaves no
// temporary state behind, which is what makes re-submitting the same file harmless.
async function postStatement(path: string, body: FormData) {
  const cookieStore = await cookies()
  const token = cookieStore.get("auth_token")?.value

  return fetch(`${env.BACKEND_URL}/api/v1/point-batches/statement/${path}`, {
    method: "POST",
    headers: { Authorization: `Bearer ${token}` },
    body,
    cache: "no-store",
  })
}

// Explicit result types rather than inferred ones: these cross the server/client boundary,
// and an inferred union of object literals reaches the caller with every key optional,
// which defeats the narrowing the caller does on "error".
type PreviewResult =
  | { error: string }
  | { success: true; reconciliation: StatementReconciliation }

type CommitResult =
  | { error: string }
  | { success: true; created: number; skipped: number }

export async function previewStatementAction(formData: FormData): Promise<PreviewResult> {
  const file = formData.get("file")
  if (!(file instanceof File) || file.size === 0) {
    return { error: "Escolha o arquivo do extrato antes de continuar." }
  }

  try {
    const res = await postStatement("preview", formData)
    const json = await res.json().catch(() => null)
    if (!res.ok) {
      return { error: json?.message || "Erro ao conciliar o extrato." }
    }

    const parsed = statementReconciliationResponseSchema.safeParse(json)
    if (!parsed.success || !parsed.data.data) {
      return { error: "Resposta inesperada do servidor." }
    }
    return { success: true, reconciliation: parsed.data.data }
  } catch {
    return { error: "Erro de conexão com o servidor." }
  }
}

export async function commitStatementAction(formData: FormData): Promise<CommitResult> {
  const file = formData.get("file")
  if (!(file instanceof File) || file.size === 0) {
    return { error: "Escolha o arquivo do extrato antes de continuar." }
  }

  try {
    const res = await postStatement("commit", formData)
    const json = await res.json().catch(() => null)
    if (!res.ok) {
      return { error: json?.message || "Erro ao lançar os lotes." }
    }

    const parsed = statementCommitResponseSchema.safeParse(json)
    if (!parsed.success || !parsed.data.data) {
      return { error: "Resposta inesperada do servidor." }
    }

    revalidatePath("/dashboard/sales/inventory")
    return {
      success: true,
      created: parsed.data.data.created,
      skipped: parsed.data.data.skipped,
    }
  } catch {
    return { error: "Erro de conexão com o servidor." }
  }
}

"use server"

import { cookies } from "next/headers"
import { env } from "@/lib/env"
import type {
  LegacyImportCommitResult,
  LegacyImportPreview,
} from "@/schemas/legacy-import"
import {
  legacyImportCommitResponseSchema,
  legacyImportPreviewResponseSchema,
} from "@/schemas/legacy-import"

// Server actions for the temporary spreadsheet importer. They forward the operator's file
// to the Go API with the JWT attached, which is the panel's standard mutation path - the
// Route Handlers in app/api exist only where the browser itself has to make the call
// (a download, the SSE stream), and this is not one of those.
//
// Delete this file with the rest of the importer; see backend/internal/legacyimport.

// The file is uploaded again on every call rather than parked somewhere server-side. It
// is under a megabyte, and a temporary feature that leaves no temporary files behind is
// one less thing to clean up when it goes.
async function postLegacyImport(path: string, body: FormData) {
  const cookieStore = await cookies()
  const token = cookieStore.get("auth_token")?.value

  return fetch(`${env.BACKEND_URL}/api/v1/issuances/legacy-import/${path}`, {
    method: "POST",
    headers: { Authorization: `Bearer ${token}` },
    body,
    cache: "no-store",
  })
}

// Explicit result types rather than inferred ones: these cross the server/client
// boundary, and an inferred union of object literals reaches the caller with every key
// optional, which defeats the narrowing the caller does on "error".
type PreviewResult =
  | { error: string }
  | { success: true; preview: LegacyImportPreview }

type CommitResult =
  | { error: string }
  | { success: true; result: LegacyImportCommitResult }

export async function previewLegacyImportAction(formData: FormData): Promise<PreviewResult> {
  const file = formData.get("file")
  if (!(file instanceof File) || file.size === 0) {
    return { error: "Escolha a planilha antes de continuar." }
  }

  try {
    const res = await postLegacyImport("preview", formData)
    const json = await res.json()
    const parsed = legacyImportPreviewResponseSchema.safeParse(json)

    if (!res.ok) {
      return { error: parsed.data?.message || "Não consegui ler a planilha." }
    }
    if (!parsed.data?.data) {
      return { error: "Resposta inesperada do servidor." }
    }

    return { success: true as const, preview: parsed.data.data }
  } catch {
    return { error: "Erro de conexão com o servidor." }
  }
}

export async function commitLegacyImportAction(formData: FormData): Promise<CommitResult> {
  const file = formData.get("file")
  if (!(file instanceof File) || file.size === 0) {
    return { error: "Escolha a planilha antes de continuar." }
  }

  try {
    const res = await postLegacyImport("commit", formData)
    const json = await res.json()
    const parsed = legacyImportCommitResponseSchema.safeParse(json)

    if (!res.ok) {
      return { error: parsed.data?.message || "Não consegui importar o dia." }
    }
    if (!parsed.data?.data) {
      return { error: "Resposta inesperada do servidor." }
    }

    return { success: true as const, result: parsed.data.data }
  } catch {
    return { error: "Erro de conexão com o servidor." }
  }
}

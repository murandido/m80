"use server"

import { cookies } from "next/headers"
import { revalidatePath } from "next/cache"
import { z } from "zod"
import { env } from "@/lib/env"

// The backend answers { data: { counter } } on both routes; only the number matters here.
const counterResponseSchema = z.object({
  data: z.object({ counter: z.number() }).nullish(),
})

// Bumps the administrator's supplier-PDF export counter by one and returns the new value. Called
// right before the download so the number in the file name advances per export - kept off the GET
// that streams the PDF so a browser prefetch cannot inflate it.
export async function incrementSupplierExportCounterAction(administratorId: string) {
  const cookieStore = await cookies()
  const token = cookieStore.get("auth_token")?.value

  try {
    const res = await fetch(`${env.BACKEND_URL}/api/v1/sales/suppliers/export-counter`, {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
        Authorization: `Bearer ${token}`,
      },
      body: JSON.stringify({ administrator_id: administratorId }),
    })

    const json = await res.json().catch(() => null)
    const parsed = counterResponseSchema.safeParse(json)
    if (!res.ok || !parsed.success || !parsed.data.data) {
      return { error: json?.message || "Erro ao incrementar o contador de exportação." }
    }

    // Not revalidated on purpose: the page refetches on its own after the download, and
    // refreshing here would race the navigation that starts the download.
    return { success: true as const, counter: parsed.data.data.counter }
  } catch {
    return { error: "Erro de conexão com o servidor." }
  }
}

// Bumps several administrators' counters at once, for the "all suppliers" PDF that covers many
// administrators in one document. Called just before that download; a no-op on an empty list.
export async function bulkIncrementSupplierExportCounterAction(administratorIds: string[]) {
  if (administratorIds.length === 0) {
    return { success: true as const, updated: 0 }
  }

  const cookieStore = await cookies()
  const token = cookieStore.get("auth_token")?.value

  try {
    const res = await fetch(`${env.BACKEND_URL}/api/v1/sales/suppliers/export-counter/bulk`, {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
        Authorization: `Bearer ${token}`,
      },
      body: JSON.stringify({ administrator_ids: administratorIds }),
    })

    const json = await res.json().catch(() => null)
    if (!res.ok) {
      return { error: json?.message || "Erro ao incrementar os contadores de exportação." }
    }

    // Not revalidated on purpose: the page refetches on its own after the download, and
    // refreshing here would race the navigation that starts it.
    return { success: true as const, updated: Number(json?.data?.updated ?? 0) }
  } catch {
    return { error: "Erro de conexão com o servidor." }
  }
}

// Writes an exact value to the administrator's export counter, for correcting it or restarting
// the sequence by hand.
export async function setSupplierExportCounterAction(administratorId: string, counter: number) {
  const cookieStore = await cookies()
  const token = cookieStore.get("auth_token")?.value

  try {
    const res = await fetch(`${env.BACKEND_URL}/api/v1/sales/suppliers/export-counter`, {
      method: "PUT",
      headers: {
        "Content-Type": "application/json",
        Authorization: `Bearer ${token}`,
      },
      body: JSON.stringify({ administrator_id: administratorId, counter }),
    })

    const json = await res.json().catch(() => null)
    const parsed = counterResponseSchema.safeParse(json)
    if (!res.ok || !parsed.success || !parsed.data.data) {
      return { error: json?.message || "Erro ao gravar o contador de exportação." }
    }

    revalidatePath("/dashboard/sales/suppliers")
    return { success: true as const, counter: parsed.data.data.counter }
  } catch {
    return { error: "Erro de conexão com o servidor." }
  }
}

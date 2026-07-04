"use server"

import { cookies } from "next/headers"
import { env } from "@/lib/env"
import { createPersonSchema, deletePersonSchema, updatePersonSchema } from "@/schemas/person"
import { z } from "zod"

// The backend treats an absent key as "don't touch" (update) / "null" (create) for
// every optional field, since they're all `*string` with `omitempty`. Sending an
// empty string would instead persist as an actual empty value and skew
// Person.ComputeComplete(), so strip blank fields before building the request body.
function stripEmptyFields<T extends Record<string, string | undefined>>(
  data: T,
): Partial<T> {
  const body: Partial<T> = {}
  for (const [key, value] of Object.entries(data)) {
    if (value) {
      body[key as keyof T] = value as T[keyof T]
    }
  }
  return body
}

export async function createPersonAction(
  data: z.infer<typeof createPersonSchema.request>,
) {
  const cookieStore = await cookies()
  const token = cookieStore.get("auth_token")?.value

  const { name, ...rest } = data
  const body = { name, ...stripEmptyFields(rest) }

  try {
    const res = await fetch(`${env.BACKEND_URL}/api/v1/people`, {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
        Authorization: `Bearer ${token}`,
      },
      body: JSON.stringify(body),
    })

    const json = await res.json()
    const parsed = createPersonSchema.response.safeParse(json)

    if (!res.ok) {
      return { error: parsed.data?.message || "Erro ao cadastrar pessoa." }
    }

    return { success: true }
  } catch {
    return { error: "Erro de conexão com o servidor." }
  }
}

export async function updatePersonAction(
  personId: string,
  data: z.infer<typeof updatePersonSchema.request>,
) {
  const cookieStore = await cookies()
  const token = cookieStore.get("auth_token")?.value

  const body = stripEmptyFields(data)

  try {
    const res = await fetch(`${env.BACKEND_URL}/api/v1/people/${personId}`, {
      method: "PUT",
      headers: {
        "Content-Type": "application/json",
        Authorization: `Bearer ${token}`,
      },
      body: JSON.stringify(body),
    })

    const json = await res.json()
    const parsed = updatePersonSchema.response.safeParse(json)

    if (!res.ok) {
      return { error: parsed.data?.message || "Erro ao atualizar pessoa." }
    }

    return { success: true }
  } catch {
    return { error: "Erro de conexão com o servidor." }
  }
}

export async function deletePersonAction(personId: string) {
  const cookieStore = await cookies()
  const token = cookieStore.get("auth_token")?.value

  try {
    const res = await fetch(`${env.BACKEND_URL}/api/v1/people/${personId}`, {
      method: "DELETE",
      headers: { Authorization: `Bearer ${token}` },
    })

    const json = await res.json()
    const parsed = deletePersonSchema.response.safeParse(json)

    if (!res.ok) {
      // e.g. 422 "Não é possível excluir pessoa vinculada a uma conta"
      return { error: parsed.data?.message || "Erro ao deletar pessoa." }
    }

    return { success: true }
  } catch {
    return { error: "Erro de conexão com o servidor." }
  }
}

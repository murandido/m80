"use server"

import { cookies } from "next/headers"
import { env } from "@/lib/env"
import {
  checkNicknameAvailabilitySchema,
  createPersonNicknameSchema,
  createPersonSchema,
  deletePersonNicknameSchema,
  deletePersonSchema,
  updatePersonSchema,
} from "@/schemas/person"
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

  const { name, nicknames, ...rest } = data
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

    const personId = parsed.data?.data?.id
    if (!personId) {
      return { error: "Erro ao cadastrar pessoa." }
    }

    // Nicknames are a separate sub-resource — the person already exists at this
    // point, so create each nickname individually and report which (if any)
    // failed instead of aborting: the frontend already checks availability
    // tag-by-tag before submit, so a failure here should only happen on a rare race.
    const failedNicknames: string[] = []
    for (const nickname of nicknames ?? []) {
      const result = await createPersonNicknameAction(personId, nickname)
      if (result.error) {
        failedNicknames.push(nickname)
      }
    }

    if (failedNicknames.length > 0) {
      return { success: true as const, personId, failedNicknames }
    }

    return { success: true as const, personId }
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

export async function createPersonNicknameAction(personId: string, nickname: string) {
  const cookieStore = await cookies()
  const token = cookieStore.get("auth_token")?.value

  try {
    const res = await fetch(`${env.BACKEND_URL}/api/v1/people/${personId}/nicknames`, {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
        Authorization: `Bearer ${token}`,
      },
      body: JSON.stringify({ nickname }),
    })

    const json = await res.json()
    const parsed = createPersonNicknameSchema.response.safeParse(json)

    if (!res.ok) {
      // e.g. 422 "Apelido já está em uso"
      return { error: parsed.data?.message || "Erro ao adicionar apelido." }
    }

    return { success: true as const }
  } catch {
    return { error: "Erro de conexão com o servidor." }
  }
}

export async function deletePersonNicknameAction(personId: string, nicknameId: string) {
  const cookieStore = await cookies()
  const token = cookieStore.get("auth_token")?.value

  try {
    const res = await fetch(
      `${env.BACKEND_URL}/api/v1/people/${personId}/nicknames/${nicknameId}`,
      {
        method: "DELETE",
        headers: { Authorization: `Bearer ${token}` },
      },
    )

    const json = await res.json()
    const parsed = deletePersonNicknameSchema.response.safeParse(json)

    if (!res.ok) {
      return { error: parsed.data?.message || "Erro ao remover apelido." }
    }

    return { success: true }
  } catch {
    return { error: "Erro de conexão com o servidor." }
  }
}

// Explicit return type is required here: without it, Next.js infers a merged
// type for this "use server" export where both `error` and `available` end up
// optional, breaking `"error" in result` narrowing at call sites.
export async function checkNicknameAvailabilityAction(
  nickname: string,
): Promise<{ error: string } | { available: boolean }> {
  const cookieStore = await cookies()
  const token = cookieStore.get("auth_token")?.value

  try {
    const res = await fetch(
      `${env.BACKEND_URL}/api/v1/people/nicknames/availability?nickname=${encodeURIComponent(nickname)}`,
      { headers: { Authorization: `Bearer ${token}` } },
    )

    const json = await res.json()
    const parsed = checkNicknameAvailabilitySchema.response.safeParse(json)
    const available = parsed.success ? parsed.data.data?.available : undefined

    if (!res.ok || available === undefined) {
      return { error: "Erro ao verificar disponibilidade do apelido." }
    }

    return { available }
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

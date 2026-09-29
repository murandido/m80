"use server"

import { cookies } from "next/headers"
import { env } from "@/lib/env"
import { stripEmptyFields } from "@/lib/strip-empty-fields"
import { normalizeSearch } from "@/lib/search"
import {
  checkNicknameAvailabilitySchema,
  createPersonNicknameSchema,
  createPersonSchema,
  deletePersonNicknameSchema,
  deletePersonSchema,
  getPeopleOptionsSchema,
  mergeRowPreviewSchema,
  mergeRowSchema,
  type MergeRowSource,
  updatePersonSchema,
} from "@/schemas/person"
import { z } from "zod"

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

// Finds the person an operator meant by typing a bare name - the administrator half of
// "conta - administrador" in the issuances grid. The search runs on the server rather than
// against the people already loaded in the browser so that somebody registered a minute ago by
// another operator is found instead of duplicated.
//
// `q` already searches names and nicknames, but it matches fragments: it is a shortlist, not an
// answer. The exact comparison happens here, accent- and case-insensitively, the same way the
// grid's combobox compares what was typed against the options it lists.
export async function findPersonByNameOrNicknameAction(name: string) {
  const cookieStore = await cookies()
  const token = cookieStore.get("auth_token")?.value

  try {
    const res = await fetch(
      `${env.BACKEND_URL}/api/v1/people?view=picker&q=${encodeURIComponent(name)}`,
      { headers: { Authorization: `Bearer ${token}` }, cache: "no-store" },
    )

    const json = await res.json()
    const parsed = getPeopleOptionsSchema.response.safeParse(json)

    if (!res.ok || !parsed.success) {
      return { error: "Erro ao buscar pessoa." }
    }

    const wanted = normalizeSearch(name)
    const people = parsed.data.data ?? []

    // Nicknames first: they are unique across the whole registry, so a hit there is an identity,
    // while several people can honestly share a name. Among equals the first wins, which mirrors
    // the bot picking the oldest match instead of refusing the message.
    const byNickname = people.find((person) =>
      person.nicknames.some((entry) => normalizeSearch(entry.nickname) === wanted),
    )
    const match = byNickname ?? people.find((person) => normalizeSearch(person.name) === wanted)

    return { personId: match?.id ?? null }
  } catch {
    return { error: "Erro de conexão com o servidor." }
  }
}

// The name typed for an administrator turned into a person id, registering her if this is the
// first time anybody mentions her. Same stance as the bot: a missing registration never blocks
// an issuance, it just leaves a person for somebody to complete later.
//
// No nickname is created for her. Person nicknames are unique across the whole registry, so
// minting one from a name typed in a hurry would be a collision waiting to happen - and the name
// itself is what findPersonByNameOrNicknameAction will match on next time.
export async function resolveOrCreatePersonAction(name: string) {
  const found = await findPersonByNameOrNicknameAction(name)
  if ("error" in found) {
    return { error: found.error }
  }
  if (found.personId) {
    return { personId: found.personId }
  }

  const created = await createPersonAction({ name, email: "" })
  if ("error" in created) {
    return { error: created.error }
  }

  return { personId: created.personId }
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
      // e.g. 422 "Esta pessoa ainda é administradora das contas de ..."
      return { error: parsed.data?.message || "Erro ao excluir." }
    }

    return { success: true as const, removal: parsed.data?.data ?? null }
  } catch {
    return { error: "Erro de conexão com o servidor." }
  }
}

function mergeRowSourceParams(source: MergeRowSource): Record<string, string> {
  return source.kind === "person"
    ? { source_person_id: source.id }
    : { source_account_id: source.id }
}

export async function previewMergeRowAction(keepId: string, source: MergeRowSource) {
  const cookieStore = await cookies()
  const token = cookieStore.get("auth_token")?.value
  const query = new URLSearchParams(mergeRowSourceParams(source))

  try {
    const res = await fetch(
      `${env.BACKEND_URL}/api/v1/people/${keepId}/merge-row-preview?${query.toString()}`,
      { headers: { Authorization: `Bearer ${token}` }, cache: "no-store" },
    )

    const json = await res.json()
    const parsed = mergeRowPreviewSchema.response.safeParse(json)

    if (!res.ok || !parsed.data?.data) {
      return { error: parsed.data?.message || "Erro ao calcular a prévia." }
    }

    return { preview: parsed.data.data }
  } catch {
    return { error: "Erro de conexão com o servidor." }
  }
}

// Absorbs a whole row into keepId's: the person (or holderless account), and every loyalty
// account in a program keepId already has. One transaction on the backend and no undo, which
// is why the panel shows the preview above first.
export async function mergeRowAction(
  keepId: string,
  source: MergeRowSource,
  keepNickname: boolean,
) {
  const cookieStore = await cookies()
  const token = cookieStore.get("auth_token")?.value

  try {
    const res = await fetch(`${env.BACKEND_URL}/api/v1/people/${keepId}/merge-row`, {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
        Authorization: `Bearer ${token}`,
      },
      body: JSON.stringify({ ...mergeRowSourceParams(source), keep_nickname: keepNickname }),
    })

    const json = await res.json()
    const parsed = mergeRowSchema.response.safeParse(json)

    if (!res.ok) {
      return { error: parsed.data?.message || "Erro ao mesclar." }
    }

    return { success: true as const, warning: parsed.data?.data?.warning ?? "" }
  } catch {
    return { error: "Erro de conexão com o servidor." }
  }
}

// Sets the administrator of every account the holder has - one per holder, the way the
// backend keeps it (internal/account/administrators.go). An empty id clears it.
export async function setHolderAdministratorAction(holderId: string, administratorId: string) {
  const cookieStore = await cookies()
  const token = cookieStore.get("auth_token")?.value

  try {
    const res = await fetch(`${env.BACKEND_URL}/api/v1/people/${holderId}/administrator`, {
      method: "PUT",
      headers: {
        "Content-Type": "application/json",
        Authorization: `Bearer ${token}`,
      },
      body: JSON.stringify({ administrator_id: administratorId || null }),
    })

    if (!res.ok) {
      const json = await res.json().catch(() => null)
      return { error: json?.message || "Erro ao definir o administrador." }
    }

    return { success: true as const }
  } catch {
    return { error: "Erro de conexão com o servidor." }
  }
}

// What deleting the holder would do, without doing it - the row's confirmation reads it.
export async function previewDeletePersonAction(personId: string) {
  const cookieStore = await cookies()
  const token = cookieStore.get("auth_token")?.value

  try {
    const res = await fetch(`${env.BACKEND_URL}/api/v1/people/${personId}/delete-preview`, {
      headers: { Authorization: `Bearer ${token}` },
      cache: "no-store",
    })
    const json = await res.json()
    const parsed = deletePersonSchema.response.safeParse(json)

    if (!res.ok || !parsed.data?.data) {
      return { error: parsed.data?.message || "Erro ao calcular a prévia." }
    }
    return { removal: parsed.data.data }
  } catch {
    return { error: "Erro de conexão com o servidor." }
  }
}

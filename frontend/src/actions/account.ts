"use server"

import { cookies } from "next/headers"
import { env } from "@/lib/env"
import { stripEmptyFields } from "@/lib/strip-empty-fields"
import { createPersonAction } from "@/actions/person"
import {
  checkAccountNicknameAvailabilitySchema,
  createAccountNicknameSchema,
  createAccountSchema,
  deleteAccountNicknameSchema,
  deleteAccountSchema,
  mergeAccountPreviewSchema,
  mergeAccountSchema,
  updateAccountSchema,
} from "@/schemas/account"
import { z } from "zod"

export async function createAccountAction(
  data: z.infer<typeof createAccountSchema.request>,
) {
  const cookieStore = await cookies()
  const token = cookieStore.get("auth_token")?.value

  const { loyalty_program_id, nicknames, ...rest } = data
  const body = { loyalty_program_id, ...stripEmptyFields(rest) }

  try {
    const res = await fetch(`${env.BACKEND_URL}/api/v1/accounts`, {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
        Authorization: `Bearer ${token}`,
      },
      body: JSON.stringify(body),
    })

    const json = await res.json()
    const parsed = createAccountSchema.response.safeParse(json)

    if (!res.ok) {
      return { error: parsed.data?.message || "Erro ao cadastrar conta." }
    }

    const account = parsed.data?.data
    if (!account) {
      return { error: "Erro ao cadastrar conta." }
    }
    const accountId = account.id

    // Nicknames are a separate sub-resource — the account already exists at this
    // point, so create each nickname individually and report which (if any)
    // failed instead of aborting: the frontend already checks availability
    // tag-by-tag before submit, so a failure here should only happen on a rare race.
    const failedNicknames: string[] = []
    for (const nickname of nicknames ?? []) {
      const result = await createAccountNicknameAction(
        accountId,
        nickname,
        loyalty_program_id,
      )
      if (result.error) {
        failedNicknames.push(nickname)
      }
    }

    if (failedNicknames.length > 0) {
      return { success: true as const, accountId, account, failedNicknames }
    }

    return { success: true as const, accountId, account }
  } catch {
    return { error: "Erro de conexão com o servidor." }
  }
}

export async function updateAccountAction(
  accountId: string,
  data: z.infer<typeof updateAccountSchema.request>,
) {
  const cookieStore = await cookies()
  const token = cookieStore.get("auth_token")?.value

  const body = stripEmptyFields(data)

  try {
    const res = await fetch(`${env.BACKEND_URL}/api/v1/accounts/${accountId}`, {
      method: "PUT",
      headers: {
        "Content-Type": "application/json",
        Authorization: `Bearer ${token}`,
      },
      body: JSON.stringify(body),
    })

    const json = await res.json()
    const parsed = updateAccountSchema.response.safeParse(json)

    if (!res.ok) {
      return { error: parsed.data?.message || "Erro ao atualizar conta." }
    }

    return { success: true }
  } catch {
    return { error: "Erro de conexão com o servidor." }
  }
}

export async function createAccountNicknameAction(
  accountId: string,
  nickname: string,
  loyaltyProgramId: string,
) {
  const cookieStore = await cookies()
  const token = cookieStore.get("auth_token")?.value

  try {
    const res = await fetch(
      `${env.BACKEND_URL}/api/v1/accounts/${accountId}/nicknames`,
      {
        method: "POST",
        headers: {
          "Content-Type": "application/json",
          Authorization: `Bearer ${token}`,
        },
        body: JSON.stringify({ loyalty_program_id: loyaltyProgramId, nickname }),
      },
    )

    const json = await res.json()
    const parsed = createAccountNicknameSchema.response.safeParse(json)

    if (!res.ok) {
      // e.g. 422 "Apelido já está em uso neste programa"
      return { error: parsed.data?.message || "Erro ao adicionar apelido." }
    }

    // The created nickname is returned so callers that need to reference it right
    // away - the issuances grid writes account_nickname_id onto the row - don't have
    // to refetch the account to learn the id.
    return { success: true as const, nickname: parsed.data?.data }
  } catch {
    return { error: "Erro de conexão com o servidor." }
  }
}

export async function deleteAccountNicknameAction(
  accountId: string,
  nicknameId: string,
) {
  const cookieStore = await cookies()
  const token = cookieStore.get("auth_token")?.value

  try {
    const res = await fetch(
      `${env.BACKEND_URL}/api/v1/accounts/${accountId}/nicknames/${nicknameId}`,
      {
        method: "DELETE",
        headers: { Authorization: `Bearer ${token}` },
      },
    )

    const json = await res.json()
    const parsed = deleteAccountNicknameSchema.response.safeParse(json)

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
export async function checkAccountNicknameAvailabilityAction(
  nickname: string,
  loyaltyProgramId: string,
): Promise<{ error: string } | { available: boolean }> {
  const cookieStore = await cookies()
  const token = cookieStore.get("auth_token")?.value

  try {
    const res = await fetch(
      `${env.BACKEND_URL}/api/v1/accounts/nicknames/availability?nickname=${encodeURIComponent(nickname)}&loyalty_program_id=${encodeURIComponent(loyaltyProgramId)}`,
      { headers: { Authorization: `Bearer ${token}` } },
    )

    const json = await res.json()
    const parsed = checkAccountNicknameAvailabilitySchema.response.safeParse(json)
    const available = parsed.success ? parsed.data.data?.available : undefined

    if (!res.ok || available === undefined) {
      return { error: "Erro ao verificar disponibilidade do apelido." }
    }

    return { available }
  } catch {
    return { error: "Erro de conexão com o servidor." }
  }
}

export async function deleteAccountAction(accountId: string) {
  const cookieStore = await cookies()
  const token = cookieStore.get("auth_token")?.value

  try {
    const res = await fetch(`${env.BACKEND_URL}/api/v1/accounts/${accountId}`, {
      method: "DELETE",
      headers: { Authorization: `Bearer ${token}` },
    })

    const json = await res.json()
    const parsed = deleteAccountSchema.response.safeParse(json)

    if (!res.ok) {
      // e.g. 422 "Não é possível excluir conta vinculada a uma emissão"
      return { error: parsed.data?.message || "Erro ao deletar conta." }
    }

    return { success: true }
  } catch {
    return { error: "Erro de conexão com o servidor." }
  }
}

// Creates the Person/Account/nickname trio behind a name typed straight into the
// issuances grid, where the operator has a name and no time to leave the spreadsheet
// to register it. Account has no name of its own - the name lives on the Person that
// holds it, and the nickname is what the grid's dropdown lists - so one typed string
// has to become three records.
//
// Orchestrated server-side so the grid spends a single round trip on it, and so the
// three calls fail in one place. There is no transaction across them: a Person whose
// account then fails to create is left behind, and the caller is told so it can say
// as much. That is the cost of doing this without a dedicated backend endpoint.
export async function createAccountForIssuanceAction(name: string, loyaltyProgramId: string) {
  // The empty strings are how these schemas spell "not provided" - the union accepts
  // "" and stripEmptyFields drops the key before it reaches the backend.
  const person = await createPersonAction({ name, email: "" })
  if ("error" in person) {
    return { error: person.error }
  }

  const account = await createAccountAction({
    loyalty_program_id: loyaltyProgramId,
    holder_id: person.personId,
    administrator_id: "",
  })
  if ("error" in account) {
    return { error: `${account.error} A pessoa "${name}" foi criada e ficou sem conta.` }
  }

  const nickname = await createAccountNicknameAction(account.accountId, name, loyaltyProgramId)
  if ("error" in nickname || !nickname.nickname) {
    const reason = "error" in nickname ? nickname.error : "Erro ao adicionar apelido."
    return { error: `${reason} A conta foi criada sem apelido.` }
  }

  // The account was created before the nickname existed, so the body echoed back by
  // POST /accounts has an empty nicknames array. The grid keys its options by nickname
  // id, so the freshly created one is folded in here rather than refetching.
  return {
    success: true as const,
    account: { ...account.account, nicknames: [nickname.nickname] },
    nicknameId: nickname.nickname.id,
  }
}

export async function previewMergeAccountAction(keepId: string, sourceId: string) {
  const cookieStore = await cookies()
  const token = cookieStore.get("auth_token")?.value

  try {
    const res = await fetch(
      `${env.BACKEND_URL}/api/v1/accounts/${keepId}/merge-preview?source_id=${sourceId}`,
      { headers: { Authorization: `Bearer ${token}` }, cache: "no-store" },
    )

    const json = await res.json()
    const parsed = mergeAccountPreviewSchema.response.safeParse(json)

    if (!res.ok || !parsed.data?.data) {
      return { error: parsed.data?.message || "Erro ao calcular a prévia." }
    }

    return { preview: parsed.data.data }
  } catch {
    return { error: "Erro de conexão com o servidor." }
  }
}

// Absorbs sourceId into keepId and deletes it. Unlike the delete button, which only
// archives, this one takes the row out - which is why the panel shows the preview first.
export async function mergeAccountAction(keepId: string, sourceId: string) {
  const cookieStore = await cookies()
  const token = cookieStore.get("auth_token")?.value

  try {
    const res = await fetch(`${env.BACKEND_URL}/api/v1/accounts/${keepId}/merge`, {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
        Authorization: `Bearer ${token}`,
      },
      body: JSON.stringify({ source_id: sourceId }),
    })

    const json = await res.json()
    const parsed = mergeAccountSchema.response.safeParse(json)

    if (!res.ok) {
      // e.g. 422 "Só é possível mesclar contas do mesmo programa"
      return { error: parsed.data?.message || "Erro ao mesclar as contas." }
    }

    return { success: true as const }
  } catch {
    return { error: "Erro de conexão com o servidor." }
  }
}

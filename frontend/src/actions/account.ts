"use server"

import { cookies } from "next/headers"
import { env } from "@/lib/env"
import { stripEmptyFields } from "@/lib/strip-empty-fields"
import {
  createPersonAction,
  createPersonNicknameAction,
  deletePersonAction,
  findPersonByNameOrNicknameAction,
  resolveOrCreatePersonAction,
} from "@/actions/person"
import { formatAccountAlias } from "@/components/issuances/account-entry"
import {
  accountBeneficiaryMutationSchema,
  getAccountAdministratorSchema,
  getAccountBeneficiariesSchema,
  createAccountNicknameSchema,
  createAccountSchema,
  deleteAccountSchema,
  updateAccountSchema,
} from "@/schemas/account"
import { createPersonSchema } from "@/schemas/person"
import { z } from "zod"

export async function createAccountAction(
  data: z.infer<typeof createAccountSchema.request>,
) {
  const cookieStore = await cookies()
  const token = cookieStore.get("auth_token")?.value

  // These are pulled out of stripEmptyFields on purpose: that helper drops every falsy
  // value, and `false` and `0` are real answers here, not empty fields. Left inside,
  // turning the exemption off would send nothing and the account would stay exempt - and a
  // profit split of zero, which means "the profit stays whole with whoever funded the
  // batch", would silently become no split at all.
  const {
    loyalty_program_id,
    nicknames,
    waives_redemption_fee,
    owns_point_inventory,
    tracks_statement,
    default_profit_share_percent,
    ...rest
  } = data
  const body = {
    loyalty_program_id,
    waives_redemption_fee: waives_redemption_fee ?? false,
    owns_point_inventory: owns_point_inventory ?? false,
    tracks_statement: tracks_statement ?? false,
    ...(default_profit_share_percent === undefined
      ? {}
      : { default_profit_share_percent }),
    ...stripEmptyFields(rest),
  }

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

// "Cadastrar Conta" registers the holder and their first loyalty account together - the
// panel no longer has a separate screen for people. Without an account (no program picked)
// only the person is created, which is how an administrator who holds nothing gets in.
//
// There is no backend transaction across the two, so the order is chosen to make undoing
// cheap: the person goes in bare, then the account, and only then the aliases. If the account
// is refused, the person is deleted again - nothing references them yet, and their aliases,
// the one thing that could, have not been written.
export async function createHolderWithAccountAction(
  holder: z.infer<typeof createPersonSchema.request>,
  account: Omit<z.infer<typeof createAccountSchema.request>, "holder_id"> | null,
) {
  const { nicknames: holderNicknames, ...holderData } = holder

  const person = await createPersonAction(holderData)
  if ("error" in person) {
    return { error: person.error }
  }

  const failedNicknames: string[] = []

  if (account) {
    const created = await createAccountAction({ ...account, holder_id: person.personId })
    if ("error" in created) {
      const rollback = await deletePersonAction(person.personId)
      return {
        error:
          "error" in rollback
            ? `${created.error} O titular "${holder.name}" foi criado e ficou sem conta.`
            : created.error,
      }
    }
    failedNicknames.push(...(created.failedNicknames ?? []))
  }

  for (const nickname of holderNicknames ?? []) {
    const result = await createPersonNicknameAction(person.personId, nickname)
    if (result.error) {
      failedNicknames.push(nickname)
    }
  }

  return { success: true as const, personId: person.personId, failedNicknames }
}

export async function updateAccountAction(
  accountId: string,
  data: z.infer<typeof updateAccountSchema.request>,
) {
  const cookieStore = await cookies()
  const token = cookieStore.get("auth_token")?.value

  // Same reason as in createAccountAction: `false` and `0` must survive the strip.
  const {
    waives_redemption_fee,
    owns_point_inventory,
    tracks_statement,
    default_profit_share_percent,
    ...rest
  } = data
  const body = {
    ...stripEmptyFields(rest),
    ...(waives_redemption_fee === undefined ? {} : { waives_redemption_fee }),
    ...(owns_point_inventory === undefined ? {} : { owns_point_inventory }),
    ...(tracks_statement === undefined ? {} : { tracks_statement }),
    ...(default_profit_share_percent === undefined ? {} : { default_profit_share_percent }),
  }

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

async function createAccountNicknameAction(
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
// `administratorName` is the half after the dash in "conta - administrador", the shape the bot
// already accepts on WhatsApp. It is resolved first, before anything is written: an administrator
// that cannot be resolved should leave the registry untouched rather than half-built.
//
// Orchestrated server-side so the grid spends a single round trip on it, and so the
// calls fail in one place. There is no transaction across them: a Person whose
// account then fails to create is left behind, and the caller is told so it can say
// as much. That is the cost of doing this without a dedicated backend endpoint.
export async function createAccountForIssuanceAction(
  name: string,
  loyaltyProgramId: string,
  administratorName?: string | null,
) {
  let administratorId = ""
  if (administratorName) {
    const administrator = await resolveOrCreatePersonAction(administratorName)
    if ("error" in administrator) {
      return { error: administrator.error }
    }
    administratorId = administrator.personId
  }

  // The empty strings are how these schemas spell "not provided" - the union accepts
  // "" and stripEmptyFields drops the key before it reaches the backend.
  const person = await createPersonAction({ name, email: "" })
  if ("error" in person) {
    return { error: person.error }
  }

  const account = await createAccountAction({
    loyalty_program_id: loyaltyProgramId,
    holder_id: person.personId,
    administrator_id: administratorId,
  })
  if ("error" in account) {
    return { error: `${account.error} A pessoa "${name}" foi criada e ficou sem conta.` }
  }

  // The saved alias keeps the administrator in it ("Marlon - Wilmara"), the same shape the
  // operator typed, rather than the bare handle before the dash.
  const alias = formatAccountAlias(name, administratorName ?? null)
  const nickname = await createAccountNicknameAction(account.accountId, alias, loyaltyProgramId)
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

// Attaches an administrator to an account that already exists, which is what
// "conta - administrador" means when the alias before the dash is already registered. The bot
// does the same on line 4 of an EMISSAO (attachAdministrator, backend/internal/bot/emissao_resolve.go).
//
// Never overwrites: an account that already answers to somebody else keeps that somebody, and the
// caller is handed the conflict to explain. The one silent case is the account whose administrator
// is the very person that was typed, reached through a different alias - there is nothing to do
// and nothing worth saying.
//
// The administrator is read from GET /accounts/{id} because view=picker does not carry it, and
// widening that feed for one flow would put a field on every account of every picker in the panel.
export async function attachAdministratorToAccountAction(
  accountId: string,
  administratorName: string,
) {
  const cookieStore = await cookies()
  const token = cookieStore.get("auth_token")?.value

  let currentAdministratorId: string | null
  try {
    const res = await fetch(`${env.BACKEND_URL}/api/v1/accounts/${accountId}`, {
      headers: { Authorization: `Bearer ${token}` },
      cache: "no-store",
    })

    const json = await res.json()
    const parsed = getAccountAdministratorSchema.response.safeParse(json)

    if (!res.ok || !parsed.success || !parsed.data.data) {
      return { error: "Erro ao ler a conta para vincular o administrador." }
    }

    currentAdministratorId = parsed.data.data.administrator_id ?? null
  } catch {
    return { error: "Erro de conexão com o servidor." }
  }

  if (currentAdministratorId) {
    // Resolved without creating anybody: the account is already spoken for, so a name nobody
    // answers to is a conflict to report, not a person to register.
    const typed = await findPersonByNameOrNicknameAction(administratorName)
    if ("error" in typed) {
      return { error: typed.error }
    }
    if (typed.personId === currentAdministratorId) {
      return { attached: false as const }
    }
    return { conflict: true as const, currentAdministratorId }
  }

  const administrator = await resolveOrCreatePersonAction(administratorName)
  if ("error" in administrator) {
    return { error: administrator.error }
  }

  // The empty strings are this schema's "leave it alone": stripEmptyFields drops them inside
  // updateAccountAction, so the PUT carries the administrator and nothing else.
  const updated = await updateAccountAction(accountId, {
    loyalty_program_id: "",
    login: "",
    password: "",
    holder_id: "",
    administrator_id: administrator.personId,
  })
  if ("error" in updated) {
    return { error: updated.error }
  }

  return { attached: true as const }
}

// --- Beneficiaries: the manual half of the CPF control ---------------------------------
//
// The automatic half derives the ledger from the issuances, and where no document arrived
// it matches passengers by name - which on the bot path is a surname. So the list needs to
// be correctable by hand, and these are the corrections.

const beneficiariesUrl = (accountId: string) =>
  `${env.BACKEND_URL}/api/v1/accounts/${accountId}/beneficiaries`

async function authHeaders() {
  const cookieStore = await cookies()
  return {
    "Content-Type": "application/json",
    Authorization: `Bearer ${cookieStore.get("auth_token")?.value}`,
  }
}

export async function getAccountBeneficiariesAction(accountId: string) {
  try {
    const res = await fetch(beneficiariesUrl(accountId), {
      headers: await authHeaders(),
      cache: "no-store",
    })
    const json = await res.json()
    const parsed = getAccountBeneficiariesSchema.response.safeParse(json)

    if (!res.ok || !parsed.success) {
      return { error: parsed.data?.message || "Erro ao buscar os beneficiários." }
    }
    return { beneficiaries: parsed.data.data ?? [] }
  } catch {
    return { error: "Erro de conexão com o servidor." }
  }
}

// The CPF is sent whole and comes back only as a mask: the API derives a fingerprint from
// it and discards the number, which is why nothing here ever reads a full document back.
export async function createAccountBeneficiaryAction(
  accountId: string,
  data: { name: string; cpf?: string },
) {
  return beneficiaryMutation(beneficiariesUrl(accountId), "POST", data, "Erro ao adicionar o beneficiário.")
}

export async function updateAccountBeneficiaryAction(
  accountId: string,
  beneficiaryId: string,
  data: { name?: string; cpf?: string; is_holder?: boolean },
) {
  return beneficiaryMutation(
    `${beneficiariesUrl(accountId)}/${beneficiaryId}`,
    "PATCH",
    data,
    "Erro ao atualizar o beneficiário.",
  )
}

export async function deleteAccountBeneficiaryAction(accountId: string, beneficiaryId: string) {
  return beneficiaryMutation(
    `${beneficiariesUrl(accountId)}/${beneficiaryId}`,
    "DELETE",
    undefined,
    "Erro ao remover o beneficiário.",
  )
}

// Moves sourceId's issuances onto beneficiaryId and deletes it - the fix for one person
// who entered twice under two spellings.
export async function mergeAccountBeneficiariesAction(
  accountId: string,
  beneficiaryId: string,
  sourceId: string,
) {
  return beneficiaryMutation(
    `${beneficiariesUrl(accountId)}/${beneficiaryId}/merge`,
    "POST",
    { source_id: sourceId },
    "Erro ao juntar os beneficiários.",
  )
}

// Admin-only at the API, and irreversible: it throws away every manual correction and
// rebuilds the list from the issuances.
export async function reconcileAccountBeneficiariesAction(accountId: string) {
  return beneficiaryMutation(
    `${beneficiariesUrl(accountId)}/reconcile`,
    "POST",
    undefined,
    "Erro ao reconstruir os beneficiários.",
  )
}

async function beneficiaryMutation(
  url: string,
  method: string,
  body: unknown,
  fallbackMessage: string,
) {
  try {
    const res = await fetch(url, {
      method,
      headers: await authHeaders(),
      ...(body === undefined ? {} : { body: JSON.stringify(body) }),
    })
    const json = await res.json()
    const parsed = accountBeneficiaryMutationSchema.response.safeParse(json)

    if (!res.ok) {
      // The API answers 400 with a readable reason for an invalid CPF, which is the one
      // the operator actually needs to see.
      return { error: parsed.data?.message || fallbackMessage }
    }
    return { success: true as const }
  } catch {
    return { error: "Erro de conexão com o servidor." }
  }
}

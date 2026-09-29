"use server"

import { cookies } from "next/headers"
import { env } from "@/lib/env"
import {
  createUserSchema,
  updateUserSchema,
  type UpdateMeRequest,
} from "@/schemas/user"
import { z } from "zod"

export async function updateUserAction(
  userId: string,
  data: z.infer<typeof updateUserSchema.request>,
) {
  const cookieStore = await cookies()
  const token = cookieStore.get("auth_token")?.value

  const body: Record<string, string | string[]> = {
    name: data.name,
    phone: data.phone,
    role: data.role,
  }
  if (data.password) {
    body.password = data.password
  }
  // Absent means "leave the grants alone" to the API, which is what an admin's edit needs.
  if (data.agency_ids) {
    body.agency_ids = data.agency_ids
  }

  try {
    const res = await fetch(`${env.BACKEND_URL}/api/v1/users/${userId}`, {
      method: "PUT",
      headers: {
        "Content-Type": "application/json",
        Authorization: `Bearer ${token}`,
      },
      body: JSON.stringify(body),
    })

    if (res.status === 403) {
      return { error: "Só desenvolvedores alteram agências, o perfil de desenvolvedor ou contas de desenvolvedor." }
    }
    if (!res.ok) {
      return { error: "Erro ao atualizar usuário." }
    }

    return { success: true }
  } catch {
    return { error: "Erro de conexão com o servidor." }
  }
}

export async function deleteUserAction(userId: string) {
  const cookieStore = await cookies()
  const token = cookieStore.get("auth_token")?.value

  try {
    const res = await fetch(`${env.BACKEND_URL}/api/v1/users/${userId}`, {
      method: "DELETE",
      headers: { Authorization: `Bearer ${token}` },
    })

    if (res.status === 403) {
      return { error: "Só desenvolvedores removem um desenvolvedor." }
    }
    if (!res.ok) {
      return { error: "Erro ao deletar usuário." }
    }

    return { success: true }
  } catch {
    return { error: "Erro de conexão com o servidor." }
  }
}

export async function createUserAction(
  data: z.infer<typeof createUserSchema.request>,
) {
  const cookieStore = await cookies()
  const token = cookieStore.get("auth_token")?.value

  try {
    const res = await fetch(`${env.BACKEND_URL}/api/v1/users`, {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
        Authorization: `Bearer ${token}`,
      },
      body: JSON.stringify(data),
    })

    const json = await res.json()
    const parsed = createUserSchema.response.safeParse(json)

    if (!res.ok) {
      return { error: parsed.data?.message || "Erro ao cadastrar usuário." }
    }

    return { success: true }
  } catch {
    return { error: "Erro de conexão com o servidor." }
  }
}

export async function updateMeAction(data: UpdateMeRequest) {
  const cookieStore = await cookies()
  const token = cookieStore.get("auth_token")?.value

  const body: Record<string, string> = {
    name: data.name,
    phone: data.phone,
  }
  if (data.new_password) {
    body.new_password = data.new_password
  }
  if (data.current_password) {
    body.current_password = data.current_password
  }

  try {
    const res = await fetch(`${env.BACKEND_URL}/api/v1/auth/me`, {
      method: "PUT",
      headers: {
        "Content-Type": "application/json",
        Authorization: `Bearer ${token}`,
      },
      body: JSON.stringify(body),
    })

    if (res.status === 403) {
      return { error: "Senha atual incorreta." }
    }
    if (res.status === 422) {
      return { error: "Este telefone já está em uso por outro usuário." }
    }
    if (res.status === 400) {
      return { error: "Telefone inválido." }
    }
    if (!res.ok) {
      return { error: "Erro ao atualizar seu perfil." }
    }

    return { success: true }
  } catch {
    return { error: "Erro de conexão com o servidor." }
  }
}

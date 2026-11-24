"use server"

import { cookies } from "next/headers"
import type { z } from "zod"
import { env } from "@/lib/env"
import {
  createMCPTokenSchema,
  decideMCPProposalSchema,
  listMCPCallsSchema,
  listMCPProposalsSchema,
  listMCPReportsSchema,
  listMCPTokensSchema,
  revokeMCPTokenSchema,
} from "@/schemas/mcp"
import type { MCPCall, MCPProposal, MCPReport, MCPToken } from "@/schemas/mcp"
import { getUsersSchema } from "@/schemas/user"
import type { User } from "@/schemas/user"

type Envelope = z.ZodType<{ success: boolean; message: string; data?: unknown }>

// callApi is the shape every action below repeats: the session cookie as a Bearer header, the
// envelope parsed, and the API's own message surfaced on failure.
async function callApi<S extends Envelope>(
  path: string,
  schema: S,
  errorMessage: string,
  init: RequestInit = {},
): Promise<{ error: string } | { data: NonNullable<z.infer<S>["data"]> }> {
  const token = (await cookies()).get("auth_token")?.value

  try {
    const res = await fetch(`${env.BACKEND_URL}/api/v1${path}`, {
      ...init,
      headers: {
        ...init.headers,
        Authorization: `Bearer ${token}`,
        "Content-Type": "application/json",
      },
      cache: "no-store",
    })
    const parsed = schema.safeParse(await res.json())

    if (!res.ok || !parsed.success || parsed.data.data == null) {
      return { error: (parsed.success && parsed.data.message) || errorMessage }
    }
    return { data: parsed.data.data as NonNullable<z.infer<S>["data"]> }
  } catch {
    return { error: "Erro de conexão com o servidor." }
  }
}

export async function listMCPTokensAction(): Promise<{ error: string } | { data: MCPToken[] }> {
  return callApi("/mcp-tokens", listMCPTokensSchema.response, "Erro ao listar tokens MCP.")
}

export async function createMCPTokenAction(input: z.input<typeof createMCPTokenSchema.request>) {
  const request = createMCPTokenSchema.request.safeParse(input)
  if (!request.success) return { error: request.error.issues[0]?.message ?? "Dados inválidos." }
  const form = request.data

  const limits =
    form.max_batch_points || form.max_batches_per_day
      ? { max_batch_points: form.max_batch_points ?? null, max_batches_per_day: form.max_batches_per_day ?? null }
      : null

  return callApi("/mcp-tokens", createMCPTokenSchema.response, "Erro ao criar token MCP.", {
    method: "POST",
    body: JSON.stringify({
      name: form.name,
      user_id: form.user_id || null,
      // Unrestricted is null, not every tool listed: a tool added later is then allowed too,
      // which is what a person's own token wants.
      allowed_write_tools: form.restricted ? form.allowed_write_tools : null,
      limits,
    }),
  })
}

// The staff a token can belong to. The API leaves statement clients out of this list by
// default, and the backend refuses them as token owners anyway.
export async function listTokenOwnersAction(): Promise<{ error: string } | { data: User[] }> {
  return callApi("/users", getUsersSchema.response, "Erro ao listar usuários.")
}

export async function listMCPReportsAction(): Promise<{ error: string } | { data: MCPReport[] }> {
  return callApi("/mcp-reports?limit=30", listMCPReportsSchema.response, "Erro ao listar relatórios MCP.")
}

export async function revokeMCPTokenAction(id: string) {
  return callApi(`/mcp-tokens/${encodeURIComponent(id)}`, revokeMCPTokenSchema.response, "Erro ao revogar token MCP.", {
    method: "DELETE",
  })
}

export async function listMCPProposalsAction(
  status: "pending" | "all",
): Promise<{ error: string } | { data: MCPProposal[] }> {
  return callApi(`/mcp-proposals?status=${status}`, listMCPProposalsSchema.response, "Erro ao listar propostas MCP.")
}

export async function decideMCPProposalAction(id: string, decision: "approve" | "reject") {
  return callApi(
    `/mcp-proposals/${encodeURIComponent(id)}/${decision}`,
    decideMCPProposalSchema.response,
    decision === "approve" ? "Erro ao aprovar proposta." : "Erro ao rejeitar proposta.",
    { method: "POST" },
  )
}

export async function listMCPCallsAction(): Promise<{ error: string } | { data: MCPCall[] }> {
  return callApi("/mcp-calls?limit=100", listMCPCallsSchema.response, "Erro ao listar chamadas MCP.")
}

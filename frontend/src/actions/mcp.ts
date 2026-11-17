"use server"

import { cookies } from "next/headers"
import type { z } from "zod"
import { env } from "@/lib/env"
import {
  createMCPTokenSchema,
  decideMCPProposalSchema,
  listMCPCallsSchema,
  listMCPProposalsSchema,
  listMCPTokensSchema,
  revokeMCPTokenSchema,
} from "@/schemas/mcp"
import type { MCPCall, MCPProposal, MCPToken } from "@/schemas/mcp"

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

export async function createMCPTokenAction(input: z.infer<typeof createMCPTokenSchema.request>) {
  const request = createMCPTokenSchema.request.safeParse(input)
  if (!request.success) return { error: "Nome inválido." }

  return callApi("/mcp-tokens", createMCPTokenSchema.response, "Erro ao criar token MCP.", {
    method: "POST",
    body: JSON.stringify(request.data),
  })
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

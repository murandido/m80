import type { Metadata } from "next"
import { cookies } from "next/headers"
import { requireDev } from "@/lib/session"
import { env } from "@/lib/env"
import { getMcpTokensSchema, McpToken } from "@/schemas/mcp-token"
import { McpTokensTable } from "@/components/mcp-tokens/mcp-tokens-table"

export const metadata: Metadata = {
  title: "Tokens MCP",
  description: "As chaves que deixam o Claude Code consultar o M80 em seu nome.",
}

export default async function McpTokensPage() {
  // Dev only, like the API routes behind it: a token reads the panel's data as its owner.
  await requireDev()

  const token = (await cookies()).get("auth_token")?.value

  let tokens: McpToken[] = []

  try {
    const res = await fetch(`${env.BACKEND_URL}/api/v1/mcp-tokens`, {
      headers: { Authorization: `Bearer ${token}` },
      cache: "no-store",
    })
    const parsed = getMcpTokensSchema.response.safeParse(await res.json())
    if (parsed.success && parsed.data.data) {
      tokens = parsed.data.data
    }
  } catch {
    // silently fail — list stays empty
  }

  return (
    <div className="p-6 sm:p-8">
      <div className="flex flex-col gap-6">
        <h1 className="text-lg font-semibold text-slate-800">Tokens MCP</h1>
        <McpTokensTable tokens={tokens} />
      </div>
    </div>
  )
}

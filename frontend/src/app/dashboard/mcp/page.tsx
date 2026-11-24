import type { Metadata } from "next"
import { requireDev } from "@/lib/session"
import {
  listMCPCallsAction,
  listMCPProposalsAction,
  listMCPReportsAction,
  listMCPTokensAction,
  listTokenOwnersAction,
} from "@/actions/mcp"
import { MCPProposalHistory, MCPProposalsCard } from "@/components/mcp/mcp-proposals-card"
import { MCPTokensCard } from "@/components/mcp/mcp-tokens-card"
import { MCPActivityCard } from "@/components/mcp/mcp-activity-card"
import { MCPReportsCard } from "@/components/mcp/mcp-reports-card"

export const metadata: Metadata = {
  title: "Agente MCP",
  description: "Propostas, tokens e atividade do agente que cuida do estoque de pontos e das emissões.",
}

export default async function MCPPage() {
  await requireDev()

  const [pendingResult, allResult, tokensResult, callsResult, reportsResult, ownersResult] = await Promise.all([
    listMCPProposalsAction("pending"),
    listMCPProposalsAction("all"),
    listMCPTokensAction(),
    listMCPCallsAction(),
    listMCPReportsAction(),
    listTokenOwnersAction(),
  ])
  const pending = "data" in pendingResult ? pendingResult.data : []
  const all = "data" in allResult ? allResult.data : []
  const tokens = "data" in tokensResult ? tokensResult.data : []
  const calls = "data" in callsResult ? callsResult.data : []
  const reports = "data" in reportsResult ? reportsResult.data : []
  // Staff only: a token carries its owner's whole authority, and the API refuses anyone else.
  const owners = "data" in ownersResult ? ownersResult.data.filter((user) => user.role !== "statement_client") : []

  return (
    <div className="p-6 sm:p-8">
      <div className="flex flex-col gap-6">
        <h1 className="text-lg font-semibold text-slate-800">Agente MCP</h1>
        <MCPReportsCard reports={reports} />
        <MCPProposalsCard proposals={pending} />
        <MCPProposalHistory proposals={all} />
        <MCPTokensCard tokens={tokens} owners={owners} />
        <MCPActivityCard calls={calls} />
      </div>
    </div>
  )
}

import type { Metadata } from "next"
import { requireDev } from "@/lib/session"
import { listMCPCallsAction, listMCPProposalsAction, listMCPTokensAction } from "@/actions/mcp"
import { MCPProposalHistory, MCPProposalsCard } from "@/components/mcp/mcp-proposals-card"
import { MCPTokensCard } from "@/components/mcp/mcp-tokens-card"
import { MCPActivityCard } from "@/components/mcp/mcp-activity-card"

export const metadata: Metadata = {
  title: "Agente MCP",
  description: "Propostas, tokens e atividade do agente que cuida do estoque de pontos e das emissões.",
}

export default async function MCPPage() {
  await requireDev()

  const [pendingResult, allResult, tokensResult, callsResult] = await Promise.all([
    listMCPProposalsAction("pending"),
    listMCPProposalsAction("all"),
    listMCPTokensAction(),
    listMCPCallsAction(),
  ])
  const pending = "data" in pendingResult ? pendingResult.data : []
  const all = "data" in allResult ? allResult.data : []
  const tokens = "data" in tokensResult ? tokensResult.data : []
  const calls = "data" in callsResult ? callsResult.data : []

  return (
    <div className="p-6 sm:p-8">
      <div className="flex flex-col gap-6">
        <h1 className="text-lg font-semibold text-slate-800">Agente MCP</h1>
        <MCPProposalsCard proposals={pending} />
        <MCPProposalHistory proposals={all} />
        <MCPTokensCard tokens={tokens} />
        <MCPActivityCard calls={calls} />
      </div>
    </div>
  )
}

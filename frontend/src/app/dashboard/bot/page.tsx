import type { Metadata } from "next"
import { requireAdmin } from "@/lib/session"
import { getBotStatusAction } from "@/actions/bot"
import {
  getMonitoredListenedChatsAction,
  getPendingListenedChatsAction,
} from "@/actions/listened-chats"
import { BotStatusCard } from "@/components/bot/bot-status-card"
import { PendingChatsTable } from "@/components/bot/pending-chats-table"
import { MonitoredChatsTable } from "@/components/bot/monitored-chats-table"

export const metadata: Metadata = {
  title: "Bot",
  description:
    "A conexão do bot com o WhatsApp e o que ele registrou em cada conversa.",
}

export default async function BotPage() {
  await requireAdmin()

  const [statusResult, pendingChatsResult, monitoredChatsResult] = await Promise.all([
    getBotStatusAction(),
    getPendingListenedChatsAction(),
    getMonitoredListenedChatsAction(),
  ])
  const initialStatus = "status" in statusResult ? statusResult.status : null
  const pendingChats = "chats" in pendingChatsResult ? pendingChatsResult.chats : []
  const monitoredChats = "chats" in monitoredChatsResult ? monitoredChatsResult.chats : []

  return (
    <div className="p-6 sm:p-8">
      <div className="flex flex-col gap-6">
        <h1 className="text-lg font-semibold text-slate-800">
          Configuração do Bot
        </h1>
        <BotStatusCard initialStatus={initialStatus} />
        <PendingChatsTable chats={pendingChats} />
        <MonitoredChatsTable chats={monitoredChats} />
      </div>
    </div>
  )
}

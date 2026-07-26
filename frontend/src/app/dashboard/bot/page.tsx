import { requireAdmin } from "@/actions/auth"
import { getBotStatusAction } from "@/actions/bot"
import { BotStatusCard } from "@/components/bot/bot-status-card"

export default async function BotPage() {
  await requireAdmin()

  const result = await getBotStatusAction()
  const initialStatus = "status" in result ? result.status : null

  return (
    <div className="p-6 sm:p-8">
      <div className="flex flex-col gap-6">
        <h1 className="text-lg font-semibold text-slate-800">
          Configuração do Bot
        </h1>
        <BotStatusCard initialStatus={initialStatus} />
      </div>
    </div>
  )
}

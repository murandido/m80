import { verifySession } from "@/actions/auth"

export default async function DashboardPage() {
  await verifySession()

  return <div>Deshiboardi</div>
}

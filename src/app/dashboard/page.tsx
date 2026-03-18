import { verifySession } from "@/actions/auth"

export default async function DashboardPage() {
  await verifySession()

export default function DashboardPage() {
  return <div>Deshiboardi</div>
}

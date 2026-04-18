import { verifySession } from "@/actions/auth"
import Link from "next/link"

export default async function DashboardPage() {
  await verifySession()

  return (
    <div>
      <p>Deshiboardi</p>
      <Link href="/dashboard/users">Usuarios</Link>
    </div>
  )
}

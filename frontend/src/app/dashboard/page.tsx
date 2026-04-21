import Link from "next/link"

export default async function DashboardPage() {
  return (
    <div>
      <p>Deshiboardi</p>
      <Link href="/dashboard/users">Usuarios</Link>
    </div>
  )
}

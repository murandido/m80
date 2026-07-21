import { getSessionUser, verifySession } from "@/actions/auth"
import { DashboardShell } from "@/components/layout/dashboard-shell"

export default async function DashboardLayout({
  children,
}: Readonly<{ children: React.ReactNode }>) {
  await verifySession()

  const user = await getSessionUser()
  const isAdmin = user?.role === "admin"

  return (
    <DashboardShell isAdmin={isAdmin} user={user}>
      {children}
    </DashboardShell>
  )
}

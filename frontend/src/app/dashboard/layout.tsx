import { getSessionUser, verifySession } from "@/lib/session"
import { getSystemStatusAction } from "@/actions/status"
import { DashboardShell } from "@/components/layout/dashboard-shell"

export default async function DashboardLayout({
  children,
}: Readonly<{ children: React.ReactNode }>) {
  await verifySession()

  const [user, statusResult] = await Promise.all([
    getSessionUser(),
    getSystemStatusAction(),
  ])
  const isAdmin = user?.role === "admin"
  // A failed status fetch leaves the lights grey, never breaks the page: the
  // indicator is the least important thing on any screen it appears on.
  const systemStatus = "status" in statusResult ? statusResult.status : null

  return (
    <DashboardShell isAdmin={isAdmin} user={user} systemStatus={systemStatus}>
      {children}
    </DashboardShell>
  )
}

import { redirect } from "next/navigation"
import { getSessionUser, verifySession } from "@/lib/session"
import { getSystemStatusAction } from "@/actions/status"
import { DashboardShell } from "@/components/layout/dashboard-shell"

export default async function DashboardLayout({
  children,
}: Readonly<{ children: React.ReactNode }>) {
  await verifySession()

  // A statement client has nothing here: every staff route answers them 403. Sent to their
  // own area before anything else renders - including the login redirect, which lands here.
  const user = await getSessionUser()
  if (user?.role === "statement_client") {
    redirect("/portal")
  }

  const statusResult = await getSystemStatusAction()
  // A failed status fetch leaves the lights grey, never breaks the page: the
  // indicator is the least important thing on any screen it appears on.
  const systemStatus = "status" in statusResult ? statusResult.status : null

  return (
    <DashboardShell role={user?.role} user={user} systemStatus={systemStatus}>
      {children}
    </DashboardShell>
  )
}

import { verifySession } from "@/actions/auth"
import { DashboardShell } from "@/components/layout/dashboard-shell"

export default async function DashboardLayout({
  children,
}: Readonly<{ children: React.ReactNode }>) {
  await verifySession()

  return <DashboardShell>{children}</DashboardShell>
}

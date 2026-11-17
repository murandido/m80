import Link from "next/link"
import { LogOut } from "lucide-react"
import { requireRole } from "@/lib/session"

// The statement client's area. Deliberately not the dashboard shell: no sidebar, no command
// palette, no status lights - nothing that names a staff screen. requireRole sends staff back
// to /dashboard; the API refuses them on /portal anyway.
export default async function PortalLayout({
  children,
}: Readonly<{ children: React.ReactNode }>) {
  const user = await requireRole("statement_client")

  return (
    <div className="min-h-screen bg-white">
      <header className="border-b border-slate-200">
        <div className="mx-auto flex w-full max-w-4xl items-center justify-between gap-4 px-4 py-3">
          <Link href="/portal" className="text-sm font-semibold text-slate-800">
            M80 · Extrato
          </Link>
          <div className="flex min-w-0 items-center gap-3 text-sm text-slate-500">
            {user && <span className="truncate">{user.name}</span>}
            <a
              href="/api/logout"
              className="flex items-center gap-1 rounded-md px-2 py-1 text-slate-600 hover:bg-slate-100"
            >
              <LogOut className="h-4 w-4" />
              Sair
            </a>
          </div>
        </div>
      </header>
      <main>{children}</main>
    </div>
  )
}

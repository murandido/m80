import type { Metadata } from "next"
import { verifySession } from "@/lib/session"
import { LatamCallback } from "@/components/accounts/latam-callback"

export const metadata: Metadata = {
  title: "Conectando a LATAM",
  description: "Conclui a conexão de uma conta LATAM com a resposta do login.",
}

// Where the handler installed on the operator's computer sends LATAM's answer to a login. The
// work happens in the client component: the answer can arrive after a "#", which never reaches
// the server.
export default async function LatamCallbackPage() {
  await verifySession()
  return (
    <div className="p-6 sm:p-8">
      <LatamCallback />
    </div>
  )
}

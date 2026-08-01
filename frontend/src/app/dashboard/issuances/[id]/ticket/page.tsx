import { cookies } from "next/headers"
import Link from "next/link"
import { AlertTriangle, ArrowLeft, Download } from "lucide-react"
import { env } from "@/lib/env"
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert"
import { Button } from "@/components/ui/button"

// The backend renders the ticket, not this page. It is the same document the PDF
// is made from, so the preview here cannot drift from the file the passenger
// receives - which is the entire reason there is one generator and not two.
async function fetchTicket(id: string): Promise<{ html: string } | { error: string }> {
  const cookieStore = await cookies()
  const token = cookieStore.get("auth_token")?.value

  try {
    const res = await fetch(`${env.BACKEND_URL}/api/v1/issuances/${id}/ticket`, {
      headers: { Authorization: `Bearer ${token}` },
      cache: "no-store",
    })

    if (res.status === 422) {
      return {
        error:
          "Essa emissão ainda não tem os dados da Traveos, então não há bilhete para mostrar. Use “Repuxar da Traveos” na grid de emissões.",
      }
    }
    if (res.status === 404) {
      return { error: "Emissão não encontrada." }
    }
    if (!res.ok) {
      return { error: "Não foi possível carregar o bilhete." }
    }

    return { html: await res.text() }
  } catch {
    return { error: "Erro de conexão com o servidor." }
  }
}

export default async function TicketPage({ params }: { params: Promise<{ id: string }> }) {
  const { id } = await params
  const result = await fetchTicket(id)

  return (
    <div className="mx-auto flex w-full max-w-[1040px] flex-col gap-4 p-4">
      <div className="flex items-center justify-between gap-4">
        <Button asChild variant="ghost" size="sm">
          <Link href="/dashboard/issuances">
            <ArrowLeft className="size-4" />
            Voltar para emissões
          </Link>
        </Button>

        {"html" in result && (
          // A plain anchor, not a fetch: the Route Handler answers with
          // Content-Disposition: attachment, and letting the browser handle the
          // download is what gives the file the name the backend chose.
          <Button asChild size="sm">
            <a href={`/api/issuances/${id}/pdf`} download>
              <Download className="size-4" />
              Baixar PDF
            </a>
          </Button>
        )}
      </div>

      {"error" in result ? (
        <Alert variant="destructive">
          <AlertTriangle className="size-4" />
          <AlertTitle>Bilhete indisponível</AlertTitle>
          <AlertDescription>{result.error}</AlertDescription>
        </Alert>
      ) : (
        // The document brings its own stylesheet, so it is rendered inside an
        // iframe rather than injected into the page: its `body` and `:root` rules
        // would otherwise apply to the whole dashboard, and its own layout would
        // inherit the dashboard's. srcDoc keeps it same-origin-free - the frame
        // gets no access to this page and needs none.
        <iframe
          title="Bilhete"
          srcDoc={result.html}
          sandbox=""
          className="h-[1400px] w-full rounded-lg border bg-white"
        />
      )}
    </div>
  )
}

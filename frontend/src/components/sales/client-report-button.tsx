"use client"

import { useState } from "react"
import { FileDown } from "lucide-react"
import { toast } from "sonner"
import { Button } from "@/components/ui/button"
import { Spinner } from "@/components/ui/spinner"

// The file name the backend chose, from Content-Disposition - it names the client when the
// statement is one client's, which is the one that gets sent.
function filenameFrom(disposition: string | null): string {
  const match = disposition?.match(/filename="?([^"]+)"?/)
  return match?.[1] ?? "controle-pagamentos.pdf"
}

// Downloads the client payment statement for the list on screen. Fetched rather than a
// plain <a download>: a refusal (nothing in the filter, PDF service down) has to become a
// toast, not a file called "report" holding a JSON error.
export function ClientReportButton({ query }: { query: string }) {
  const [busy, setBusy] = useState(false)

  async function handleClick() {
    setBusy(true)
    try {
      const res = await fetch(`/api/sales/clients/report?${query}`)
      if (!res.ok) {
        const body = await res.json().catch(() => null)
        toast.error(body?.error || "Não foi possível gerar o PDF.")
        return
      }
      const url = URL.createObjectURL(await res.blob())
      const link = document.createElement("a")
      link.href = url
      link.download = filenameFrom(res.headers.get("Content-Disposition"))
      link.click()
      URL.revokeObjectURL(url)
    } catch {
      toast.error("Erro de conexão com o servidor.")
    } finally {
      setBusy(false)
    }
  }

  return (
    <Button
      type="button"
      variant="outline"
      disabled={busy}
      onClick={() => void handleClick()}
      title="PDF das emissões filtradas, com o que já foi pago e o que falta, para enviar ao cliente"
    >
      {busy ? <Spinner className="h-4 w-4" /> : <FileDown className="h-4 w-4" />}
      PDF do cliente
    </Button>
  )
}

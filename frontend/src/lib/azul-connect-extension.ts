// The panel side of connecting an Azul account through the operator's own browser - the fallback
// for when the server-side login cannot get through. The extension's bridge content script listens
// for these messages on the panel origin and answers through the same window; it opens the Azul
// site in a tab, the operator logs in there, and what comes back is the account key from that
// login plus the CPF it was made with. The password never comes back to the page. The string
// constants are a wire format shared with a separate repository and must match it.

const CONNECT_REQUEST = "m80:azul-connect"
const CONNECT_RESULT = "m80:azul-connect-result"
const BRIDGE_ATTRIBUTE = "data-m80-azul-connect"
// A person logs in on the airline's site, sometimes with a code on the holder's phone. The
// extension gives up at ten minutes; the page waits a little longer so it hears that answer.
const RESULT_TIMEOUT_MS = 11 * 60_000

export type AzulBrowserConnectAnswer = { customerKey: string; document: string } | { error: string }

// The bridge marks the document as soon as it runs, so the card can offer the browser path only
// when there is an extension to answer it.
export function azulConnectBridgeReady(): boolean {
  if (typeof document === "undefined") return false
  return document.documentElement.getAttribute(BRIDGE_ATTRIBUTE) === "ready"
}

export function requestAzulBrowserConnect(accountId: string, cpf: string): Promise<AzulBrowserConnectAnswer> {
  return new Promise((resolve) => {
    if (typeof window === "undefined") {
      resolve({ error: "O navegador não está disponível." })
      return
    }
    const document = cpf.replace(/\D/g, "")
    if (document.length !== 11) {
      resolve({ error: "A conta precisa do CPF salvo como login para conectar pelo navegador." })
      return
    }
    const requestId = crypto.randomUUID()
    const timer = setTimeout(() => {
      cleanup()
      resolve({ error: "O login na Azul não terminou a tempo. Nada foi gravado." })
    }, RESULT_TIMEOUT_MS)

    function cleanup() {
      clearTimeout(timer)
      window.removeEventListener("message", onMessage)
    }

    function onMessage(event: MessageEvent) {
      // Only our own page and origin, mirroring what the extension checks on its side.
      if (event.source !== window || event.origin !== window.location.origin) return
      const data = event.data as {
        type?: string
        requestId?: string
        customerKey?: unknown
        document?: unknown
        error?: unknown
      }
      if (!data || data.type !== CONNECT_RESULT || data.requestId !== requestId) return
      cleanup()
      if (typeof data.customerKey === "string" && data.customerKey !== "" && typeof data.document === "string") {
        resolve({ customerKey: data.customerKey, document: data.document })
        return
      }
      resolve({ error: typeof data.error === "string" ? data.error : "A extensão não devolveu a conexão." })
    }

    window.addEventListener("message", onMessage)
    window.postMessage({ type: CONNECT_REQUEST, request: { requestId, accountId, document } }, window.location.origin)
  })
}

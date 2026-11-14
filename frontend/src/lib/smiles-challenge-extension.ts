// The panel side of the Smiles challenge handshake. The extension's bridge content script
// (extension entrypoints/smiles-panel.content.ts) listens for these messages on the panel
// origin and answers through the same window. The two string constants are a wire format shared
// with a separate repository - they must match lib/smiles-challenge.ts there.

const CHALLENGE_REQUEST = "m80:smiles-challenge"
const CHALLENGE_RESULT = "m80:smiles-challenge-result"
const BRIDGE_ATTRIBUTE = "data-m80-smiles-challenge"
const RESULT_TIMEOUT_MS = 45_000

export interface SmilesChallengeMaterial {
  clientId: string
  mfaToken: string
  authenticatorId: string
}

export type SmilesChallengeAnswer = { oobCode: string } | { error: string }

// The bridge marks the document as soon as it runs, so the dialog can pick the browser path
// without waiting for a round trip that would never be answered when the extension is missing.
export function challengeBridgeReady(): boolean {
  if (typeof document === "undefined") return false
  return document.documentElement.getAttribute(BRIDGE_ATTRIBUTE) === "ready"
}

export function requestSmilesChallenge(material: SmilesChallengeMaterial): Promise<SmilesChallengeAnswer> {
  return new Promise((resolve) => {
    if (typeof window === "undefined") {
      resolve({ error: "O navegador não está disponível." })
      return
    }
    const requestId = crypto.randomUUID()
    const timer = setTimeout(() => {
      cleanup()
      resolve({ error: "A extensão não respondeu a tempo. Tente de novo." })
    }, RESULT_TIMEOUT_MS)

    function cleanup() {
      clearTimeout(timer)
      window.removeEventListener("message", onMessage)
    }

    function onMessage(event: MessageEvent) {
      // Only our own page and origin, mirroring what the extension checks on its side.
      if (event.source !== window || event.origin !== window.location.origin) return
      const data = event.data as { type?: string; requestId?: string; oobCode?: unknown; error?: unknown }
      if (!data || data.type !== CHALLENGE_RESULT || data.requestId !== requestId) return
      cleanup()
      if (typeof data.oobCode === "string" && data.oobCode !== "") {
        resolve({ oobCode: data.oobCode })
        return
      }
      resolve({ error: typeof data.error === "string" ? data.error : "A extensão não devolveu o código." })
    }

    window.addEventListener("message", onMessage)
    window.postMessage({ type: CHALLENGE_REQUEST, request: { requestId, ...material } }, window.location.origin)
  })
}

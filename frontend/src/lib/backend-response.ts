// Turns a failed backend response into the message a server action shows the operator.
//
// The actions used to call `res.json()` before looking at the status, so any reply that is
// not JSON threw and surfaced as "Erro de conexão com o servidor." - with the server up.
// Two such replies are routine: the API's rate limiter (httprate) answers 429 in plain
// text, and chi's Recoverer answers 500 with no body after a panic. Both sent people
// looking at the network instead of waiting a few seconds or at the API logs.

interface ReadActionErrorOptions {
  // Some routes word their 5xx messages in Portuguese on purpose (the Traveos sync answers
  // 502/503 with a sentence meant for the operator). Everywhere else a 5xx message is a raw
  // `err.Error()` in English, so by default it is replaced by the fallback.
  keepServerErrorMessage?: boolean
}

// Reads the body as JSON without throwing: a non-JSON reply becomes null, so the caller's
// own fallback applies instead of the connection-error catch.
export async function readBackendJson(res: Response): Promise<unknown> {
  return res.json().catch(() => null)
}

function messageOf(body: unknown): string | undefined {
  if (body && typeof body === "object" && "message" in body) {
    const message = (body as { message: unknown }).message
    if (typeof message === "string" && message.trim() !== "") return message
  }
  return undefined
}

export async function readActionError(
  res: Response,
  fallback: string,
  options: ReadActionErrorOptions = {},
): Promise<string> {
  if (res.status === 429) {
    return "Muitas requisições, aguarde alguns segundos e tente de novo."
  }
  // Every 401 on these routes comes from the auth middleware (missing, expired or revoked
  // token); its English "Access denied: ..." tells the operator nothing they can act on.
  if (res.status === 401) {
    return "Sua sessão expirou. Entre de novo para continuar."
  }

  const serverMessage = messageOf(await readBackendJson(res))

  if (res.status >= 500) {
    if (options.keepServerErrorMessage && serverMessage) return serverMessage
    return `${fallback} O servidor teve um problema; tente de novo em instantes.`
  }

  return serverMessage || fallback
}

// What the operator typed into the account cell, once split into the account alias and the
// administrator it should answer to. The bot accepts the same "conta - administrador" shape on
// line 4 of an EMISSAO, so an operator who learned it on WhatsApp can type it here too.
export interface AccountEntry {
  nickname: string
  administratorName: string | null
}

// The bot splits on a bare "-" (splitTrimmed, backend/internal/bot/emissao_parse.go), which costs
// it aliases that contain a dash. That trade does not work here: on WhatsApp the alias is a short
// handle, while in the grid it is routinely the holder's full name - and "Ana Souza-Lima" must stay
// one alias. Requiring the spaces keeps both readings possible, at the cost of "Ana-Wilmara"
// being read as a plain alias instead of a pairing.
const SEPARATOR = " - "

// A dash left hanging at either end is the operator mid-keystroke ("Heraldo - " with nobody after
// it yet), and neither an alias nor a name ever legitimately starts or ends with one - so it is
// dropped rather than registered as part of the name.
function trimDanglingDash(value: string): string {
  return value.replace(/^-+|-+$/g, "").trim()
}

// The inverse of parseAccountEntry's split: joins an alias and its administrator back into the
// "conta - administrador" shape. Used when creating an account from the grid so the saved alias
// keeps the administrator in it ("Marlon - Wilmara") rather than the bare handle.
export function formatAccountAlias(nickname: string, administratorName: string | null): string {
  return administratorName ? `${nickname}${SEPARATOR}${administratorName}` : nickname
}

export function parseAccountEntry(typed: string): AccountEntry {
  const text = typed.trim()
  const at = text.indexOf(SEPARATOR)
  if (at === -1) {
    return { nickname: trimDanglingDash(text), administratorName: null }
  }

  const nickname = text.slice(0, at).trim()
  // Only two fields, like the bot: anything after a second separator is dropped instead of
  // becoming part of the administrator's name. The dropdown entry spells out the name it will
  // link, so what was ignored is visible before the operator accepts it.
  const rest = text.slice(at + SEPARATOR.length)
  const secondSeparator = rest.indexOf(SEPARATOR)
  const administratorName = (secondSeparator === -1 ? rest : rest.slice(0, secondSeparator)).trim()

  if (nickname === "" || administratorName === "") {
    return { nickname: trimDanglingDash(text), administratorName: null }
  }

  return { nickname, administratorName }
}

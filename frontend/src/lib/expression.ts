// Arithmetic typed straight into a grid cell.
//
// Operators reach for a calculator to add two taxes, double a milheiro or split points
// across legs, then type the answer back. These two functions let the cell do that work:
// "189,22+56,50" resolves to 245.72 before it is saved.
//
// Deliberately hand-written rather than `eval` or a library. `eval` on operator-typed text
// is a code path nobody should have, and a dependency for four operators does not pay for
// itself - the grammar below is the whole feature.
//
//   expr   := term (('+' | '-') term)*
//   term   := factor (('*' | '/') factor)*
//   factor := '-'? ( number | '(' expr ')' )
//   number := \d+([.,]\d+)?
//
// Inside an expression a dot and a comma are both decimal separators, so "189.22*2" and
// "189,22*2" agree. That is the one place this grammar departs from how the cells read a
// bare number, where a dot groups thousands - which is why the caller shows the result
// before it is committed.

const NUMBER = /^\d+(?:[.,]\d+)?/

// looksLikeExpression reports whether the text is a sum to resolve rather than a number to
// read. A leading minus is excluded on purpose: "-50" is a negative amount, the way it has
// always been, while "31-5" is a subtraction.
export function looksLikeExpression(text: string): boolean {
  const trimmed = text.trim()
  if (trimmed === "") return false
  if (/[+*/()]/.test(trimmed)) return true
  // A minus anywhere but the front - "31-5" yes, "-50" no.
  return trimmed.indexOf("-", 1) > 0
}

// evaluateExpression resolves the text to a number, or null when it does not parse: a
// dangling operator, an unclosed bracket, a stray token, a division by zero, or anything
// that overflows to Infinity. The caller treats null as "leave the cell alone", so a
// half-typed sum can never overwrite a stored value.
export function evaluateExpression(text: string): number | null {
  const tokens = tokenize(text)
  if (tokens === null) return null

  const parser = new Parser(tokens)
  const value = parser.parseExpression()
  // Anything left over means the text was not one expression - "1 2" or "(1+2))".
  if (value === null || !parser.done()) return null
  if (!Number.isFinite(value)) return null

  return value
}

type Token = { kind: "number"; value: number } | { kind: "operator"; value: string }

function tokenize(text: string): Token[] | null {
  const tokens: Token[] = []
  let rest = text.trim()

  while (rest !== "") {
    if (rest[0] === " ") {
      rest = rest.slice(1)
      continue
    }

    if ("+-*/()".includes(rest[0])) {
      tokens.push({ kind: "operator", value: rest[0] })
      rest = rest.slice(1)
      continue
    }

    const match = NUMBER.exec(rest)
    if (match === null) return null
    tokens.push({ kind: "number", value: Number(match[0].replace(",", ".")) })
    rest = rest.slice(match[0].length)
  }

  return tokens
}

// A recursive-descent parser over the token list. Every level returns null on a malformed
// input and the callers propagate it, so one bad token anywhere aborts the whole parse.
class Parser {
  private index = 0

  constructor(private readonly tokens: Token[]) {}

  done(): boolean {
    return this.index >= this.tokens.length
  }

  parseExpression(): number | null {
    let left = this.parseTerm()
    if (left === null) return null

    while (this.peekOperator("+") || this.peekOperator("-")) {
      const operator = this.take().value
      const right = this.parseTerm()
      if (right === null) return null
      left = operator === "+" ? left + right : left - right
    }

    return left
  }

  private parseTerm(): number | null {
    let left = this.parseFactor()
    if (left === null) return null

    while (this.peekOperator("*") || this.peekOperator("/")) {
      const operator = this.take().value
      const right = this.parseFactor()
      if (right === null) return null
      // Dividing by zero is a typo, not a value: null keeps the stored number.
      if (operator === "/" && right === 0) return null
      left = operator === "*" ? left * right : left / right
    }

    return left
  }

  private parseFactor(): number | null {
    if (this.peekOperator("-")) {
      this.take()
      const value = this.parseFactor()
      return value === null ? null : -value
    }

    const token = this.tokens[this.index]
    if (token === undefined) return null

    if (token.kind === "number") {
      this.index += 1
      return token.value
    }

    if (token.value !== "(") return null
    this.index += 1
    const inner = this.parseExpression()
    if (inner === null) return null
    if (!this.peekOperator(")")) return null
    this.index += 1

    return inner
  }

  private peekOperator(value: string): boolean {
    const token = this.tokens[this.index]
    return token !== undefined && token.kind === "operator" && token.value === value
  }

  private take(): Token {
    const token = this.tokens[this.index]
    this.index += 1
    return token
  }
}

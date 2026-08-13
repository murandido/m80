// Folds a string down to what a search should actually compare: no surrounding space, no
// case, no diacritics. Operators type names without accents ("joao" for "João"), and a
// strict comparison both hides the existing record and clears the "already exists" guard
// that would otherwise block a duplicate - which is how two spellings of one client end
// up in the database. NFD splits each accented letter into base + combining mark, so
// dropping the combining range (U+0300-U+036F) covers every accent Portuguese uses.
export function normalizeSearch(value: string): string {
  return value
    .trim()
    .toLowerCase()
    .normalize("NFD")
    .replace(/[̀-ͯ]/g, "")
}

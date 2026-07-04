// The backend treats an absent key as "don't touch" (update) / "null" (create) for
// every optional field, since they're all `*string` with `omitempty`. Sending an
// empty string would instead persist as an actual empty value and skew server-side
// "complete" computations, so strip blank fields before building the request body.
export function stripEmptyFields<T extends Record<string, string | undefined>>(
  data: T,
): Partial<T> {
  const body: Partial<T> = {}
  for (const [key, value] of Object.entries(data)) {
    if (value) {
      body[key as keyof T] = value as T[keyof T]
    }
  }
  return body
}

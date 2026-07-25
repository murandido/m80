"use client"

import { useEffect, useRef } from "react"

export type SSEHandlers = Record<string, (data: unknown) => void>

// Subscribes to a server-sent events endpoint and dispatches each named event to the
// matching handler. Deliberately domain-agnostic: it knows nothing about issuances, so
// other screens (the bot connection status, later) can point it at the same endpoint.
//
// Reconnection is left to the native EventSource, which already retries with a backoff
// after the server or the platform closes the stream.
export function useSSE(url: string, handlers: SSEHandlers): void {
  // Handlers are read through a ref so that a caller passing a fresh object literal on
  // every render doesn't tear the connection down and open a new one each time.
  const handlersRef = useRef(handlers)
  useEffect(() => {
    handlersRef.current = handlers
  })

  // Only the event names decide which listeners to register, so the effect re-runs when
  // they change - not when the callbacks' identities do.
  const eventNames = Object.keys(handlers).sort().join(",")

  useEffect(() => {
    const source = new EventSource(url)

    const listeners = eventNames.split(",").map((eventName) => {
      const listener = (event: MessageEvent<string>) => {
        let payload: unknown = null
        try {
          payload = JSON.parse(event.data)
        } catch {
          // A malformed frame is not worth breaking the stream over - the next
          // event will arrive normally.
          return
        }
        handlersRef.current[eventName]?.(payload)
      }

      source.addEventListener(eventName, listener)
      return { eventName, listener }
    })

    return () => {
      for (const { eventName, listener } of listeners) {
        source.removeEventListener(eventName, listener)
      }
      source.close()
    }
  }, [url, eventNames])
}

"use client"

import { useEffect, useRef } from "react"

export type SSEHandlers = Record<string, (data: unknown) => void>

// Subscribes to a server-sent events endpoint and dispatches each named event to the
// matching handler. Deliberately domain-agnostic: it knows nothing about issuances, so
// other screens (the bot connection status, later) can point it at the same endpoint.
//
// Reconnection is left to the native EventSource, which already retries with a backoff
// after the server or the platform closes the stream.
//
// What EventSource cannot do is replay what was broadcast while it was away - a deploy,
// a restarted panel, a dropped network - so a screen that only refetches on events stays
// quietly out of date after every reconnect, until someone presses F5. `onReconnect`
// runs on every open after the first, which is the caller's cue to refetch once.
export function useSSE(
  url: string,
  handlers: SSEHandlers,
  { onReconnect }: { onReconnect?: () => void } = {},
): void {
  // Handlers are read through a ref so that a caller passing a fresh object literal on
  // every render doesn't tear the connection down and open a new one each time.
  const handlersRef = useRef(handlers)
  const onReconnectRef = useRef(onReconnect)
  useEffect(() => {
    handlersRef.current = handlers
    onReconnectRef.current = onReconnect
  })

  // Only the event names decide which listeners to register, so the effect re-runs when
  // they change - not when the callbacks' identities do.
  const eventNames = Object.keys(handlers).sort().join(",")

  useEffect(() => {
    const source = new EventSource(url)

    let hasOpened = false
    const handleOpen = () => {
      if (hasOpened) onReconnectRef.current?.()
      hasOpened = true
    }
    source.addEventListener("open", handleOpen)

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
      source.removeEventListener("open", handleOpen)
      for (const { eventName, listener } of listeners) {
        source.removeEventListener(eventName, listener)
      }
      source.close()
    }
  }, [url, eventNames])
}

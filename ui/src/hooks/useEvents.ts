// ============================================================
// ui/src/hooks/useEvents.ts — Real-time event hook
//
// This React hook opens a Server-Sent Events (SSE) connection
// to /api/events and calls `onEvent` every time the server
// pushes a new message (scan started, scan finished, etc.).
//
// If the connection drops (network blip, container restart),
// it automatically reconnects — starting at 1 second, doubling
// each attempt up to a max of 30 seconds.
// ============================================================

import { useEffect, useRef } from 'react'
import type { SseEvent } from '../types'

export function useEvents(onEvent: (event: SseEvent) => void) {
  // We store the callback in a ref so changing it doesn't restart the connection.
  // Without this, every time the parent component re-renders it would disconnect
  // and reconnect the SSE stream.
  const onEventRef = useRef(onEvent)
  onEventRef.current = onEvent

  useEffect(() => {
    let es: EventSource
    let retryDelay = 1000 // start with 1 second between retries

    function connect() {
      // EventSource is a browser built-in that handles SSE connections
      es = new EventSource('/api/events')

      es.onmessage = (e: MessageEvent) => {
        retryDelay = 1000 // reset retry delay on successful message
        try {
          // Each message is a JSON string — parse it into our typed event
          const event = JSON.parse(e.data as string) as SseEvent
          onEventRef.current(event)
        } catch {
          // Ignore any malformed messages
        }
      }

      es.onerror = () => {
        // Connection dropped — close the broken connection and schedule a retry
        es.close()
        setTimeout(connect, retryDelay)
        // Exponential backoff: 1s → 2s → 4s → 8s → ... → 30s max
        retryDelay = Math.min(retryDelay * 2, 30_000)
      }
    }

    connect() // open the connection when the component mounts

    // Cleanup: close the SSE connection when the component unmounts
    return () => es?.close()
  }, []) // empty array = run once on mount, never re-run
}

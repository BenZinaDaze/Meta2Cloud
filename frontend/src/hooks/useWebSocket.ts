import { useCallback, useEffect, useRef, useState } from 'react'

export interface WsMessage {
  type: string
  ts: string | null
  level: string
  message: string
  runId?: string
}

interface UseWebSocketOptions {
  onMessage: (msg: WsMessage) => void
}

export function useWebSocket({ onMessage }: UseWebSocketOptions) {
  const [connected, setConnected] = useState(false)
  const onMessageRef = useRef(onMessage)
  onMessageRef.current = onMessage
  const wsRef = useRef<WebSocket | null>(null)
  const reconnectRef = useRef<ReturnType<typeof setTimeout> | null>(null)
  const mountedRef = useRef(false)

  const connect = useCallback(() => {
    if (wsRef.current && (wsRef.current.readyState === WebSocket.OPEN || wsRef.current.readyState === WebSocket.CONNECTING)) return

    const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:'
    const token = localStorage.getItem('auth_token') || ''
    const wsUrl = `${protocol}//${window.location.host}/api/logs/live?token=${encodeURIComponent(token)}`

    const ws = new WebSocket(wsUrl)
    wsRef.current = ws

    ws.onopen = () => {
      if (mountedRef.current) setConnected(true)
    }

    ws.onmessage = (event) => {
      try {
        onMessageRef.current(JSON.parse(event.data))
      } catch {
        // ignore parse errors
      }
    }

    ws.onclose = () => {
      if (wsRef.current === ws) wsRef.current = null
      if (mountedRef.current) {
        setConnected(false)
        reconnectRef.current = setTimeout(connect, 3000)
      }
    }

    ws.onerror = () => {
      ws.close()
    }
  }, [])

  useEffect(() => {
    mountedRef.current = true
    connect()
    return () => {
      mountedRef.current = false
      if (reconnectRef.current) clearTimeout(reconnectRef.current)
    }
  }, [connect])

  return { connected }
}

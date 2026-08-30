"use client"

import { useEffect, useRef, useState } from "react"
import { useTranslation } from "@/components/i18n-provider"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"
import { Loader2, Monitor, AlertCircle, Maximize2, Minimize2 } from "lucide-react"

let RFBClass: any = null

interface VMVNCConsoleProps {
  vmUuid: string
}

export function VMVNCConsole({ vmUuid }: VMVNCConsoleProps) {
  const { t } = useTranslation()
  const vncContainerRef = useRef<HTMLDivElement>(null)
  const rfbRef = useRef<any>(null)

  const [isConnecting, setIsConnecting] = useState(true)
  const [isConnected, setIsConnected] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [isFullscreen, setIsFullscreen] = useState(false)
  const [reconnectAttempt, setReconnectAttempt] = useState(0)
  useEffect(() => {
    if (!vncContainerRef.current || !vmUuid) return

    const initializeVNC = async () => {
      try {
        setIsConnecting(true)
        setError(null)

        if (!RFBClass) {
          const module = await import("@novnc/novnc")
          RFBClass = module.default
        }

        // Get VNC connection details
        const vncInfoResponse = await fetch(`/api/vms/${vmUuid}/vnc`, {
          credentials: 'include'
        })

        if (!vncInfoResponse.ok) {
          const errorText = await vncInfoResponse.text()
          throw new Error(`Failed to get VNC info: ${errorText}`)
        }

        const { websocket_path, token } = await vncInfoResponse.json()
        if (!websocket_path || !token) {
          throw new Error('Invalid VNC connection response')
        }

        // Construct WebSocket URL
        const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:'
        const wsUrl = `${protocol}//${window.location.host}${websocket_path}?token=${encodeURIComponent(token)}`

        // Clear container
        vncContainerRef.current!.innerHTML = ''

        // Initialize noVNC RFB client
        const rfb = new RFBClass(vncContainerRef.current, wsUrl, {
          credentials: { password: '' },
          shared: true,
        })

        // Configure RFB options
        rfb.scaleViewport = true
        rfb.resizeSession = false
        rfb.showDotCursor = true
        rfb.background = '#000000'

        // Event handlers
        rfb.addEventListener('connect', () => {
          console.log('VNC connected')
          setIsConnecting(false)
          setIsConnected(true)
          setError(null)
        })

        rfb.addEventListener('disconnect', (event: any) => {
          console.log('VNC disconnected:', event.detail)
          setIsConnecting(false)
          setIsConnected(false)

          if (event.detail.clean) {
            setError('Connection closed')
          } else {
            setError(`Disconnected: ${event.detail.reason || 'Unknown error'}`)
          }
        })

        rfb.addEventListener('credentialsrequired', () => {
          console.log('VNC credentials required')
        })

        rfb.addEventListener('securityfailure', (event: any) => {
          console.error('VNC security failure:', event.detail)
          setError(`Security failure: ${event.detail.reason || 'Authentication failed'}`)
          setIsConnecting(false)
          setIsConnected(false)
        })

        // Store reference
        rfbRef.current = rfb

      } catch (err) {
        console.error('Failed to initialize VNC:', err)
        setError(err instanceof Error ? err.message : 'Failed to initialize VNC')
        setIsConnecting(false)
        setIsConnected(false)
      }
    }

    initializeVNC()

    // Cleanup
    return () => {
      if (rfbRef.current) {
        try {
          rfbRef.current.disconnect()
        } catch (e) {
          console.error('Error disconnecting VNC:', e)
        }
        rfbRef.current = null
      }
    }
  }, [vmUuid, reconnectAttempt])

  const toggleFullscreen = () => {
    if (!vncContainerRef.current) return

    if (!document.fullscreenElement) {
      vncContainerRef.current.requestFullscreen()
      setIsFullscreen(true)
    } else {
      document.exitFullscreen()
      setIsFullscreen(false)
    }
  }

  const reconnect = () => {
    if (rfbRef.current) {
      try {
        rfbRef.current.disconnect()
      } catch (e) {
        console.error('Error during reconnect cleanup:', e)
      }
      rfbRef.current = null
    }

    setError(null)
    setIsConnecting(true)
    setIsConnected(false)

    setReconnectAttempt((attempt) => attempt + 1)
  }

  return (
    <Card>
      <CardHeader>
        <div className="flex items-center justify-between">
          <CardTitle className="flex items-center gap-2">
            <Monitor className="h-5 w-5" />
            VNC Console
          </CardTitle>
          <div className="flex gap-2">
            {isConnected && (
              <Button
                variant="outline"
                size="sm"
                onClick={toggleFullscreen}
              >
                {isFullscreen ? (
                  <><Minimize2 className="h-4 w-4 mr-2" /> Exit Fullscreen</>
                ) : (
                  <><Maximize2 className="h-4 w-4 mr-2" /> Fullscreen</>
                )}
              </Button>
            )}
            {error && (
              <Button
                variant="outline"
                size="sm"
                onClick={reconnect}
              >
                Reconnect
              </Button>
            )}
          </div>
        </div>
      </CardHeader>
      <CardContent>
        {isConnecting && (
          <div className="flex items-center justify-center h-96 bg-black rounded-md">
            <div className="flex flex-col items-center gap-2 text-white">
              <Loader2 className="h-8 w-8 animate-spin" />
              <p>Connecting to VNC...</p>
            </div>
          </div>
        )}

        {error && (
          <div className="flex items-center justify-center h-96 bg-black rounded-md">
            <div className="flex flex-col items-center gap-2 text-white">
              <AlertCircle className="h-8 w-8 text-red-500" />
              <p className="text-red-400">{error}</p>
            </div>
          </div>
        )}

        <div
          ref={vncContainerRef}
          className={`vnc-container bg-black rounded-md ${
            !isConnecting && !error ? 'block' : 'hidden'
          }`}
          style={{
            minHeight: '600px',
            width: '100%',
            position: 'relative',
          }}
        />
      </CardContent>
    </Card>
  )
}

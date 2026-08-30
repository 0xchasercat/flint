"use client"

import type React from "react"
import { useState, useEffect } from "react"
import { useTranslation } from "@/components/i18n-provider"
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card"
import { Label } from "@/components/ui/label"
import { Input } from "@/components/ui/input"
import { Button } from "@/components/ui/button"
import { Switch } from "@/components/ui/switch"
import { Alert, AlertDescription } from "@/components/ui/alert"
import { Monitor, Server, Key, CheckCircle, XCircle, Loader2 } from "lucide-react"
import { hostAPI, connectionAPI, type ConnectionStatus, type SSHKey } from "@/lib/api"
import { useToast } from "@/hooks/use-toast"

interface SystemInfo {
  hostname: string
  cpuCores: number
  totalMemory: string
  storagePath: string
}

export function SettingsView() {
  const { t } = useTranslation()
  const { toast } = useToast()
  const [systemInfo, setSystemInfo] = useState<SystemInfo>({
    hostname: "localhost",
    cpuCores: 0,
    totalMemory: "0 GB",
    storagePath: "/var/lib/libvirt",
  })

  const [isLoading, setIsLoading] = useState(true)
  const [connectionStatus, setConnectionStatus] = useState<ConnectionStatus | null>(null)
  const [sshKeys, setSSHKeys] = useState<SSHKey[]>([])

  // Connection form state
  const [sshEnabled, setSSHEnabled] = useState(false)
  const [sshUsername, setSSHUsername] = useState("")
  const [sshHost, setSSHHost] = useState("")
  const [sshPort, setSSHPort] = useState(22)
  const [sshKeyPath, setSSHKeyPath] = useState("")
  const [uri, setUri] = useState("qemu:///system")

  const [isTesting, setIsTesting] = useState(false)
  const [isSaving, setIsSaving] = useState(false)
  const [testResult, setTestResult] = useState<{ success: boolean; message: string } | null>(null)

  useEffect(() => {
    const fetchData = async () => {
      try {
        // Fetch system info
        const status = await hostAPI.getStatus()
        const resources = await hostAPI.getResources()

        setSystemInfo({
          hostname: status.hostname || "localhost",
          cpuCores: resources.cpu_cores,
          totalMemory: `${(resources.total_memory_kb / 1024 / 1024).toFixed(1)} GB`,
          storagePath: "/var/lib/libvirt",
        })

        // Fetch connection status
        const connStatus = await connectionAPI.getStatus()
        setConnectionStatus(connStatus)

        // Populate form with current connection settings
        setUri(connStatus.uri)
        setSSHEnabled(connStatus.ssh_enabled)
        if (connStatus.ssh_enabled) {
          setSSHUsername(connStatus.ssh_username || "")
          setSSHHost(connStatus.ssh_host || "")
          setSSHPort(connStatus.ssh_port || 22)
        }

        // Detect available SSH keys
        const keysResponse = await connectionAPI.detectSSHKeys()
        setSSHKeys(keysResponse.keys)
        if (keysResponse.keys.length > 0 && !sshKeyPath) {
          setSSHKeyPath(keysResponse.keys[0].path)
        }
      } catch (error) {
        console.error("Failed to fetch data:", error)
      } finally {
        setIsLoading(false)
      }
    }

    fetchData()
  }, [])

  const handleTestConnection = async () => {
    setIsTesting(true)
    setTestResult(null)

    try {
      const result = await connectionAPI.testConnection({
        uri,
        ssh_enabled: sshEnabled,
        ssh_username: sshUsername,
        ssh_host: sshHost,
        ssh_port: sshPort,
        ssh_key_path: sshKeyPath,
      })

      setTestResult(result)
      if (result.success) {
        toast({
          title: "Connection successful",
          description: result.message,
        })
      } else {
        toast({
          title: "Connection failed",
          description: result.message,
          variant: "destructive",
        })
      }
    } catch (error) {
      setTestResult({
        success: false,
        message: error instanceof Error ? error.message : "Unknown error",
      })
      toast({
        title: "Test failed",
        description: "Failed to test connection",
        variant: "destructive",
      })
    } finally {
      setIsTesting(false)
    }
  }

  const handleSaveConnection = async () => {
    setIsSaving(true)

    try {
      const result = await connectionAPI.updateConfig({
        uri,
        ssh_enabled: sshEnabled,
        ssh_username: sshUsername,
        ssh_host: sshHost,
        ssh_port: sshPort,
        ssh_key_path: sshKeyPath,
      })

      toast({
        title: "Configuration saved",
        description: result.message,
      })

      // Refresh connection status
      const newStatus = await connectionAPI.getStatus()
      setConnectionStatus(newStatus)
    } catch (error) {
      toast({
        title: "Save failed",
        description: error instanceof Error ? error.message : "Failed to save configuration",
        variant: "destructive",
      })
    } finally {
      setIsSaving(false)
    }
  }

  if (isLoading) {
    return (
      <div className="container max-w-6xl py-8 px-6 sm:px-8 md:px-10">
        <div className="flex items-center justify-center min-h-[200px]">
          <div className="text-center">
            <h2 className="text-xl font-semibold mb-2">{t('settings.loadingSystemInfo')}</h2>
          </div>
        </div>
      </div>
    )
  }

  return (
    <div className="container max-w-6xl py-8 px-6 sm:px-8 md:px-10">
      <div className="flex items-center justify-between mb-6">
        <div>
          <h1 className="text-3xl font-bold tracking-tight">{t('settings.systemInformation')}</h1>
          <p className="text-muted-foreground mt-1">{t('settings.systemInformationDesc')}</p>
        </div>
      </div>

      <div className="space-y-6">
        {/* Connection Status */}
        {connectionStatus && (
          <Alert variant={connectionStatus.connected ? "default" : "destructive"}>
            <div className="flex items-center gap-2">
              {connectionStatus.connected ? (
                <CheckCircle className="h-4 w-4" />
              ) : (
                <XCircle className="h-4 w-4" />
              )}
              <AlertDescription>
                {connectionStatus.connected
                  ? `Connected to ${connectionStatus.effective_uri}`
                  : `Disconnected: ${connectionStatus.error_message}`}
              </AlertDescription>
            </div>
          </Alert>
        )}

        {/* Connection Settings */}
        <Card>
          <CardHeader className="pb-3">
            <CardTitle className="flex items-center gap-2">
              <Server className="h-5 w-5" />
              Connection Settings
            </CardTitle>
            <CardDescription>Configure libvirt connection (local or remote via SSH)</CardDescription>
          </CardHeader>
          <CardContent className="space-y-6">
            {/* Base URI */}
            <div className="space-y-2">
              <Label>libvirt URI</Label>
              <Input
                value={uri}
                onChange={(e) => setUri(e.target.value)}
                placeholder="qemu:///system"
              />
              <p className="text-sm text-muted-foreground">
                Base URI for libvirt connection (e.g., qemu:///system or qemu:///session)
              </p>
            </div>

            {/* SSH Enable Toggle */}
            <div className="flex items-center justify-between">
              <div className="space-y-0.5">
                <Label>Enable SSH Connection</Label>
                <p className="text-sm text-muted-foreground">
                  Connect to a remote libvirt server via SSH
                </p>
              </div>
              <Switch checked={sshEnabled} onCheckedChange={setSSHEnabled} />
            </div>

            {/* SSH Configuration (shown when SSH is enabled) */}
            {sshEnabled && (
              <div className="space-y-4 pl-6 border-l-2 border-muted">
                <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
                  <div className="space-y-2">
                    <Label>SSH Username *</Label>
                    <Input
                      value={sshUsername}
                      onChange={(e) => setSSHUsername(e.target.value)}
                      placeholder="root"
                    />
                  </div>
                  <div className="space-y-2">
                    <Label>SSH Host *</Label>
                    <Input
                      value={sshHost}
                      onChange={(e) => setSSHHost(e.target.value)}
                      placeholder="192.168.1.100"
                    />
                  </div>
                  <div className="space-y-2">
                    <Label>SSH Port</Label>
                    <Input
                      type="number"
                      value={sshPort}
                      onChange={(e) => setSSHPort(Number.parseInt(e.target.value) || 22)}
                      placeholder="22"
                    />
                  </div>
                  <div className="space-y-2">
                    <Label className="flex items-center gap-2">
                      <Key className="h-4 w-4" />
                      SSH Key Path *
                    </Label>
                    {sshKeys.length > 0 ? (
                      <select
                        className="w-full px-3 py-2 border border-input bg-background rounded-md"
                        value={sshKeyPath}
                        onChange={(e) => setSSHKeyPath(e.target.value)}
                      >
                        {sshKeys.map((key) => (
                          <option key={key.path} value={key.path}>
                            {key.name} {!key.secure && "(⚠️ insecure permissions)"}
                          </option>
                        ))}
                      </select>
                    ) : (
                      <Input
                        value={sshKeyPath}
                        onChange={(e) => setSSHKeyPath(e.target.value)}
                        placeholder="~/.ssh/id_rsa"
                      />
                    )}
                  </div>
                </div>
                <p className="text-sm text-muted-foreground">
                  Ensure your SSH key is configured on the remote server and has proper permissions (600 or 400)
                </p>
              </div>
            )}

            {/* Test Result */}
            {testResult && (
              <Alert variant={testResult.success ? "default" : "destructive"}>
                <AlertDescription>{testResult.message}</AlertDescription>
              </Alert>
            )}

            {/* Action Buttons */}
            <div className="flex gap-3">
              <Button onClick={handleTestConnection} disabled={isTesting} variant="outline">
                {isTesting && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
                Test Connection
              </Button>
              <Button onClick={handleSaveConnection} disabled={isSaving}>
                {isSaving && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
                Save Configuration
              </Button>
            </div>
          </CardContent>
        </Card>

        {/* System Information */}
        <Card>
          <CardHeader className="pb-3">
            <CardTitle className="flex items-center gap-2">
              <Monitor className="h-5 w-5" />
              {t('settings.systemInformation')}
            </CardTitle>
            <CardDescription>{t('settings.systemInformationDesc')}</CardDescription>
          </CardHeader>
          <CardContent className="space-y-4">
            <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
              <div className="space-y-2">
                <Label>{t('settings.hostname')}</Label>
                <Input value={systemInfo.hostname} readOnly className="bg-muted" />
              </div>
              <div className="space-y-2">
                <Label>{t('settings.cpuCores')}</Label>
                <Input value={systemInfo.cpuCores} readOnly className="bg-muted" />
              </div>
              <div className="space-y-2">
                <Label>{t('settings.totalMemory')}</Label>
                <Input value={systemInfo.totalMemory} readOnly className="bg-muted" />
              </div>
              <div className="space-y-2">
                <Label>{t('settings.storagePath')}</Label>
                <Input value={systemInfo.storagePath} readOnly className="bg-muted" />
              </div>
            </div>
          </CardContent>
        </Card>
      </div>
    </div>
  )
}

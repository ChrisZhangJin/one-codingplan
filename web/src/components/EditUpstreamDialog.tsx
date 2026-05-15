import { useEffect, useState } from 'react'
import { toast } from 'sonner'

import { apiFetch } from '@/lib/api'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'

type Protocol = 'openai' | 'anthropic' | 'both'

export interface UpstreamInfo {
  id: number
  name: string
  base_url: string
  enabled: boolean
  available: boolean
  model_override?: string
  masked_key?: string
  protocol?: Protocol
  passthrough_extensions?: boolean
}

interface EditUpstreamDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  onUpdated: () => void
  upstream: UpstreamInfo | null
}

export default function EditUpstreamDialog({
  open,
  onOpenChange,
  onUpdated,
  upstream,
}: EditUpstreamDialogProps) {
  const [name, setName] = useState('')
  const [baseUrl, setBaseUrl] = useState('')
  const [apiKey, setApiKey] = useState('')
  const [modelOverride, setModelOverride] = useState('')
  const [protocol, setProtocol] = useState<Protocol>('both')
  const [passthrough, setPassthrough] = useState(false)
  const [submitting, setSubmitting] = useState(false)

  useEffect(() => {
    if (upstream) {
      setName(upstream.name)
      setBaseUrl(upstream.base_url)
      setApiKey('')
      setModelOverride(upstream.model_override ?? '')
      setProtocol(upstream.protocol ?? 'both')
      setPassthrough(upstream.passthrough_extensions ?? false)
    }
  }, [upstream])

  function handleClose() {
    onOpenChange(false)
  }

  async function handleSubmit() {
    if (!upstream) return
    setSubmitting(true)
    try {
      const body: Record<string, unknown> = {
        name,
        base_url: baseUrl,
        model_override: modelOverride,
        protocol,
        passthrough_extensions: protocol !== 'openai' && passthrough,
      }
      if (apiKey !== '') body.api_key = apiKey

      await apiFetch(`/api/upstreams/${upstream.id}`, {
        method: 'PATCH',
        body: JSON.stringify(body),
      })
      toast.success('Upstream updated')
      onUpdated()
      handleClose()
    } catch (err: unknown) {
      const msg = err instanceof Error ? err.message : 'Failed to update upstream'
      toast.error(msg)
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <Dialog open={open} onOpenChange={(isOpen) => { if (!isOpen) handleClose() }}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Edit Upstream</DialogTitle>
        </DialogHeader>
        <div className="flex flex-col gap-4">
          <div className="flex flex-col gap-1.5">
            <Label htmlFor="edit-upstream-name">Name</Label>
            <Input
              id="edit-upstream-name"
              value={name}
              onChange={e => setName(e.target.value)}
            />
          </div>
          <div className="flex flex-col gap-1.5">
            <Label htmlFor="edit-upstream-url">Base URL</Label>
            <Input
              id="edit-upstream-url"
              value={baseUrl}
              onChange={e => setBaseUrl(e.target.value)}
            />
          </div>
          <div className="flex flex-col gap-1.5">
            <Label htmlFor="edit-upstream-key">API Key</Label>
            <Input
              id="edit-upstream-key"
              type="password"
              placeholder={upstream?.masked_key ? `Current: ${upstream.masked_key}` : 'Leave blank to keep existing'}
              value={apiKey}
              onChange={e => setApiKey(e.target.value)}
            />
          </div>
          <div className="flex flex-col gap-1.5">
            <Label htmlFor="edit-upstream-model">Model Override</Label>
            <Input
              id="edit-upstream-model"
              placeholder="e.g. claude-3-5-sonnet-20241022"
              value={modelOverride}
              onChange={e => setModelOverride(e.target.value)}
            />
          </div>
          <div className="flex flex-col gap-1.5">
            <Label>Protocol</Label>
            <div className="flex gap-3 text-sm">
              {(['openai', 'anthropic', 'both'] as const).map(p => (
                <label key={p} className="flex items-center gap-1.5 cursor-pointer">
                  <input
                    type="radio"
                    name="edit-upstream-protocol"
                    value={p}
                    checked={protocol === p}
                    onChange={() => setProtocol(p)}
                  />
                  <span>{p === 'openai' ? 'OpenAI' : p === 'anthropic' ? 'Anthropic' : 'Both'}</span>
                </label>
              ))}
            </div>
          </div>
          {protocol !== 'openai' && (
            <div className="flex items-start gap-2">
              <input
                id="edit-upstream-passthrough"
                type="checkbox"
                checked={passthrough}
                onChange={e => setPassthrough(e.target.checked)}
                className="mt-1"
              />
              <div className="flex flex-col">
                <Label htmlFor="edit-upstream-passthrough" className="cursor-pointer">
                  Forward Claude-specific fields
                </Label>
                <span className="text-xs text-muted-foreground">
                  Enable for proxies that terminate at real Claude so <code>thinking</code> and <code>betas</code> reach the API.
                </span>
              </div>
            </div>
          )}
          <DialogFooter>
            <Button variant="outline" onClick={handleClose} disabled={submitting}>
              Cancel
            </Button>
            <Button onClick={handleSubmit} disabled={submitting}>
              Save Changes
            </Button>
          </DialogFooter>
        </div>
      </DialogContent>
    </Dialog>
  )
}

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

interface AddUpstreamDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  onCreated: () => void
}

type Protocol = 'openai' | 'anthropic' | 'both'

const PROVIDERS = ['minimax', 'mimo', 'kimi', 'qwen', 'glm', 'deepseek', 'custom'] as const
type Provider = (typeof PROVIDERS)[number]
const PROVIDER_LABELS: Record<Provider, string> = {
  minimax: 'Minimax', mimo: 'Mimo', kimi: 'Kimi', qwen: 'Qwen', glm: 'GLM', deepseek: 'DeepSeek',
  custom: 'Custom (self-hosted)',
}
const DEFAULT_PROTOCOL: Record<Provider, Protocol> = {
  minimax: 'both', mimo: 'both', kimi: 'both',
  qwen: 'openai', glm: 'openai', deepseek: 'openai',
  custom: 'anthropic',
}

const CUSTOM_SLUG_REGEX = /^[a-z0-9-]{2,32}$/

export default function AddUpstreamDialog({ open, onOpenChange, onCreated }: AddUpstreamDialogProps) {
  const [provider, setProvider] = useState<Provider | ''>('')
  const [customName, setCustomName] = useState('')
  const [baseUrl, setBaseUrl] = useState('')
  const [apiKey, setApiKey] = useState('')
  const [modelOverride, setModelOverride] = useState('')
  const [protocol, setProtocol] = useState<Protocol>('both')
  const [passthrough, setPassthrough] = useState(false)
  const [submitting, setSubmitting] = useState(false)

  // When the picked provider changes, prefill the protocol radio.
  useEffect(() => {
    if (provider) setProtocol(DEFAULT_PROTOCOL[provider])
  }, [provider])

  function resetState() {
    setProvider('')
    setCustomName('')
    setBaseUrl('')
    setApiKey('')
    setModelOverride('')
    setProtocol('both')
    setPassthrough(false)
    setSubmitting(false)
  }

  function handleClose() {
    onOpenChange(false)
    resetState()
  }

  const resolvedName = provider === 'custom' ? customName.trim() : provider

  async function handleSubmit() {
    if (!provider) {
      toast.error('Provider is required')
      return
    }
    if (provider === 'custom' && !CUSTOM_SLUG_REGEX.test(customName.trim())) {
      toast.error('Custom name must be 2–32 chars of [a-z0-9-]')
      return
    }
    if (!baseUrl.trim()) {
      toast.error('Base URL is required')
      return
    }
    if (!apiKey.trim()) {
      toast.error('API Key is required')
      return
    }
    setSubmitting(true)
    try {
      await apiFetch('/api/upstreams', {
        method: 'POST',
        body: JSON.stringify({
          name: resolvedName,
          base_url: baseUrl.trim(),
          api_key: apiKey.trim(),
          model_override: modelOverride.trim(),
          protocol,
          passthrough_extensions: protocol !== 'openai' && passthrough,
        }),
      })
      toast.success('Upstream added')
      onCreated()
      handleClose()
    } catch (err: unknown) {
      toast.error(err instanceof Error ? err.message : 'Failed to add upstream')
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <Dialog open={open} onOpenChange={(isOpen) => { if (!isOpen) handleClose() }}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Add Upstream</DialogTitle>
        </DialogHeader>
        <div className="flex flex-col gap-4">
          <div className="flex flex-col gap-2">
            <Label htmlFor="add-upstream-provider">Provider</Label>
            <select
              id="add-upstream-provider"
              value={provider}
              onChange={e => setProvider(e.target.value as Provider | '')}
              className="h-8 w-full rounded-lg border border-input bg-transparent px-2.5 py-1 text-sm outline-none focus-visible:border-ring focus-visible:ring-3 focus-visible:ring-ring/50 dark:bg-input/30"
            >
              <option value="">Select provider…</option>
              {PROVIDERS.map(p => (
                <option key={p} value={p}>{PROVIDER_LABELS[p]}</option>
              ))}
            </select>
          </div>
          {provider === 'custom' && (
            <div className="flex flex-col gap-2">
              <Label htmlFor="add-upstream-custom-name">Name</Label>
              <Input
                id="add-upstream-custom-name"
                type="text"
                placeholder="my-claude-proxy"
                required
                value={customName}
                onChange={e => setCustomName(e.target.value)}
              />
              <span className="text-xs text-muted-foreground">
                2–32 chars; lowercase letters, digits, dashes.
              </span>
            </div>
          )}
          <div className="flex flex-col gap-2">
            <Label htmlFor="add-upstream-url">Base URL</Label>
            <Input
              id="add-upstream-url"
              type="text"
              placeholder="https://api.moonshot.ai"
              required
              value={baseUrl}
              onChange={e => setBaseUrl(e.target.value)}
            />
          </div>
          <div className="flex flex-col gap-2">
            <Label htmlFor="add-upstream-key">API Key</Label>
            <Input
              id="add-upstream-key"
              type="password"
              placeholder="Paste API key"
              required
              value={apiKey}
              onChange={e => setApiKey(e.target.value)}
            />
          </div>
          <div className="flex flex-col gap-2">
            <Label htmlFor="add-upstream-model">Model Override</Label>
            <Input
              id="add-upstream-model"
              type="text"
              placeholder="e.g. claude-3-5-sonnet-20241022"
              value={modelOverride}
              onChange={e => setModelOverride(e.target.value)}
            />
          </div>
          <div className="flex flex-col gap-2">
            <Label>Protocol</Label>
            <div className="flex gap-3 text-sm">
              {(['openai', 'anthropic', 'both'] as const).map(p => (
                <label key={p} className="flex items-center gap-1.5 cursor-pointer">
                  <input
                    type="radio"
                    name="upstream-protocol"
                    value={p}
                    checked={protocol === p}
                    onChange={() => setProtocol(p)}
                  />
                  <span className="capitalize">{p === 'openai' ? 'OpenAI' : p === 'anthropic' ? 'Anthropic' : 'Both'}</span>
                </label>
              ))}
            </div>
            <span className="text-xs text-muted-foreground">
              Which API shapes the upstream speaks at this URL.
            </span>
          </div>
          {protocol !== 'openai' && (
            <div className="flex items-start gap-2">
              <input
                id="add-upstream-passthrough"
                type="checkbox"
                checked={passthrough}
                onChange={e => setPassthrough(e.target.checked)}
                className="mt-1"
              />
              <div className="flex flex-col">
                <Label htmlFor="add-upstream-passthrough" className="cursor-pointer">
                  Forward Claude-specific fields
                </Label>
                <span className="text-xs text-muted-foreground">
                  Enable for proxies that terminate at real Claude so <code>thinking</code> and <code>betas</code> reach the API. Leave off for third-party Anthropic-compatible providers.
                </span>
              </div>
            </div>
          )}
        </div>
        <DialogFooter>
          <Button variant="outline" disabled={submitting} onClick={handleClose}>
            Discard
          </Button>
          <Button disabled={submitting} onClick={handleSubmit}>
            Add Upstream
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}

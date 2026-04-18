import { useState } from 'react'
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

export default function AddUpstreamDialog({ open, onOpenChange, onCreated }: AddUpstreamDialogProps) {
  const [name, setName] = useState('')
  const [baseUrl, setBaseUrl] = useState('')
  const [apiKey, setApiKey] = useState('')
  const [modelOverride, setModelOverride] = useState('')
  const [submitting, setSubmitting] = useState(false)

  function resetState() {
    setName('')
    setBaseUrl('')
    setApiKey('')
    setModelOverride('')
    setSubmitting(false)
  }

  function handleClose() {
    onOpenChange(false)
    resetState()
  }

  async function handleSubmit() {
    if (!name.trim()) {
      toast.error('Name is required')
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
          name: name.trim(),
          base_url: baseUrl.trim(),
          api_key: apiKey.trim(),
          model_override: modelOverride.trim(),
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
            <Label htmlFor="add-upstream-name">Name</Label>
            <Input
              id="add-upstream-name"
              type="text"
              placeholder="e.g. kimi-prod"
              required
              value={name}
              onChange={e => setName(e.target.value)}
            />
          </div>
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

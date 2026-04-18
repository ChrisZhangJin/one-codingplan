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

interface KeyResponse {
  id: string
  name: string
  token: string
  enabled: boolean
  token_budget: number
  allowed_upstreams: string[]
  expires_at?: string
  rate_limit_per_minute: number
  rate_limit_per_day: number
  day_usage: number
  usage_total_input: number
  usage_total_output: number
  created_at: string
  updated_at: string
}

interface EditKeyDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  onUpdated: () => void
  keyData: KeyResponse | null
}

export default function EditKeyDialog({
  open,
  onOpenChange,
  onUpdated,
  keyData,
}: EditKeyDialogProps) {
  const [name, setName] = useState('')
  const [tokenBudget, setTokenBudget] = useState('')
  const [allowedUpstreams, setAllowedUpstreams] = useState('')
  const [expiresAt, setExpiresAt] = useState('')
  const [rateLimitPerMinute, setRateLimitPerMinute] = useState('')
  const [rateLimitPerDay, setRateLimitPerDay] = useState('')
  const [submitting, setSubmitting] = useState(false)

  useEffect(() => {
    if (keyData) {
      setName(keyData.name)
      setTokenBudget(keyData.token_budget === 0 ? '' : String(keyData.token_budget))
      setAllowedUpstreams(keyData.allowed_upstreams.join(', '))
      setExpiresAt(keyData.expires_at ? new Date(keyData.expires_at).toISOString().slice(0, 16) : '')
      setRateLimitPerMinute(keyData.rate_limit_per_minute === 0 ? '' : String(keyData.rate_limit_per_minute))
      setRateLimitPerDay(keyData.rate_limit_per_day === 0 ? '' : String(keyData.rate_limit_per_day))
    }
  }, [keyData])

  function handleClose() {
    onOpenChange(false)
  }

  async function handleSubmit() {
    if (!keyData) return
    setSubmitting(true)
    try {
      const body: Record<string, unknown> = {}
      if (name !== keyData.name) body.name = name
      const budgetVal = tokenBudget ? parseInt(tokenBudget, 10) : 0
      if (budgetVal !== keyData.token_budget) body.token_budget = budgetVal
      const upstreams = allowedUpstreams
        ? allowedUpstreams.split(',').map(s => s.trim()).filter(Boolean)
        : []
      const origUpstreams = keyData.allowed_upstreams
      if (JSON.stringify(upstreams) !== JSON.stringify(origUpstreams)) {
        body.allowed_upstreams = upstreams
      }
      if (expiresAt) {
        const newExpiry = new Date(expiresAt).toISOString()
        if (newExpiry !== keyData.expires_at) body.expires_at = newExpiry
      }
      const rpmVal = rateLimitPerMinute ? parseInt(rateLimitPerMinute, 10) : 0
      if (rpmVal !== keyData.rate_limit_per_minute) body.rate_limit_per_minute = rpmVal
      const rpdVal = rateLimitPerDay ? parseInt(rateLimitPerDay, 10) : 0
      if (rpdVal !== keyData.rate_limit_per_day) body.rate_limit_per_day = rpdVal

      await apiFetch(`/api/keys/${keyData.id}`, {
        method: 'PATCH',
        body: JSON.stringify(body),
      })
      toast.success('Key updated')
      onUpdated()
      handleClose()
    } catch (err: unknown) {
      const msg = err instanceof Error ? err.message : 'Failed to update key'
      toast.error(msg)
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <Dialog open={open} onOpenChange={(isOpen) => { if (!isOpen) handleClose() }}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Edit Access Key</DialogTitle>
        </DialogHeader>
        <div className="flex flex-col gap-4">
          {keyData && (
            <p className="text-sm text-muted-foreground font-mono">
              Token: {keyData.token}
            </p>
          )}
          <div className="flex flex-col gap-1.5">
            <Label htmlFor="edit-key-name">Name</Label>
            <Input
              id="edit-key-name"
              value={name}
              onChange={e => setName(e.target.value)}
            />
          </div>
          <div className="flex flex-col gap-1.5">
            <Label htmlFor="edit-key-budget">Token Budget</Label>
            <Input
              id="edit-key-budget"
              type="number"
              placeholder="0 = unlimited"
              value={tokenBudget}
              onChange={e => setTokenBudget(e.target.value)}
            />
          </div>
          <div className="flex flex-col gap-1.5">
            <Label htmlFor="edit-key-upstreams">Allowed Upstreams</Label>
            <Input
              id="edit-key-upstreams"
              placeholder="comma-separated, blank = all"
              value={allowedUpstreams}
              onChange={e => setAllowedUpstreams(e.target.value)}
            />
          </div>
          <div className="flex flex-col gap-1.5">
            <Label htmlFor="edit-key-expires">Expires At</Label>
            <Input
              id="edit-key-expires"
              type="datetime-local"
              value={expiresAt}
              onChange={e => setExpiresAt(e.target.value)}
            />
          </div>
          <div className="flex flex-col gap-1.5">
            <Label htmlFor="edit-key-rpm">Rate Limit/min</Label>
            <Input
              id="edit-key-rpm"
              type="number"
              placeholder="0 = unlimited"
              value={rateLimitPerMinute}
              onChange={e => setRateLimitPerMinute(e.target.value)}
            />
          </div>
          <div className="flex flex-col gap-1.5">
            <Label htmlFor="edit-key-rpd">Rate Limit/day</Label>
            <Input
              id="edit-key-rpd"
              type="number"
              placeholder="0 = unlimited"
              value={rateLimitPerDay}
              onChange={e => setRateLimitPerDay(e.target.value)}
            />
          </div>
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

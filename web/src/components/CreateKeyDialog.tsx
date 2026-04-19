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

interface CreateKeyDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  onCreated: () => void
}

function parseTokenBudget(s: string): number | null {
  const t = s.trim()
  if (!t) return null
  const m = t.match(/^(\d+(?:\.\d+)?)\s*([km]?)$/i)
  if (!m) return null
  const n = parseFloat(m[1])
  const suffix = m[2].toLowerCase()
  if (suffix === 'k') return Math.round(n * 1_000)
  if (suffix === 'm') return Math.round(n * 1_000_000)
  return Math.round(n)
}

function copyText(text: string) {
  if (navigator.clipboard) {
    navigator.clipboard.writeText(text)
    return
  }
  const el = document.createElement('textarea')
  el.value = text
  el.style.position = 'fixed'
  el.style.opacity = '0'
  document.body.appendChild(el)
  el.select()
  document.execCommand('copy')
  document.body.removeChild(el)
}

export default function CreateKeyDialog({ open, onOpenChange, onCreated }: CreateKeyDialogProps) {
  const [name, setName] = useState('')
  const [tokenBudget, setTokenBudget] = useState('')
  const [allowedUpstreams, setAllowedUpstreams] = useState('')
  const [expiresAt, setExpiresAt] = useState('')
  const [rateLimitPerMinute, setRateLimitPerMinute] = useState('')
  const [rateLimitPerDay, setRateLimitPerDay] = useState('')
  const [createdToken, setCreatedToken] = useState<string | null>(null)
  const [submitting, setSubmitting] = useState(false)

  function resetState() {
    setName('')
    setTokenBudget('')
    setAllowedUpstreams('')
    setExpiresAt('')
    setRateLimitPerMinute('')
    setRateLimitPerDay('')
    setCreatedToken(null)
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
    setSubmitting(true)
    try {
      const body: Record<string, unknown> = { name: name.trim() }
      if (tokenBudget) {
        const budget = parseTokenBudget(tokenBudget)
        if (budget === null) {
          toast.error('Invalid token budget — use a number like 100000, 100k, or 1.5M')
          setSubmitting(false)
          return
        }
        body.token_budget = budget
      }
      if (allowedUpstreams) body.allowed_upstreams = allowedUpstreams.split(',').map(s => s.trim()).filter(Boolean)
      if (expiresAt) body.expires_at = new Date(expiresAt).toISOString()
      if (rateLimitPerMinute) body.rate_limit_per_minute = parseInt(rateLimitPerMinute, 10)
      if (rateLimitPerDay) body.rate_limit_per_day = parseInt(rateLimitPerDay, 10)

      const result = await apiFetch<{ token: string }>('/api/keys', {
        method: 'POST',
        body: JSON.stringify(body),
      })
      setCreatedToken(result.token)
      toast.success("Key created. Copy it now — it won't be shown again.")
      onCreated()
    } catch (err: unknown) {
      const msg = err instanceof Error ? err.message : 'Failed to create key'
      toast.error(msg)
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <Dialog open={open} onOpenChange={(isOpen) => { if (!isOpen) handleClose() }}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Create Access Key</DialogTitle>
        </DialogHeader>

        {createdToken !== null ? (
          <div className="flex flex-col gap-4">
            <p className="text-sm text-muted-foreground">
              Key created. Copy it now -- it won't be shown again.
            </p>
            <Input readOnly value={createdToken} className="font-mono text-xs" />
            <DialogFooter>
              <Button
                variant="outline"
                onClick={() => {
                  copyText(createdToken)
                  toast.success('Copied to clipboard')
                }}
              >
                Copy
              </Button>
              <Button onClick={handleClose}>Close</Button>
            </DialogFooter>
          </div>
        ) : (
          <div className="flex flex-col gap-4">
            <div className="flex flex-col gap-1.5">
              <Label htmlFor="key-name">Name</Label>
              <Input
                id="key-name"
                placeholder="e.g. claude-code-laptop"
                value={name}
                onChange={e => setName(e.target.value)}
              />
            </div>
            <div className="flex flex-col gap-1.5">
              <Label htmlFor="key-budget">Token Budget</Label>
              <Input
                id="key-budget"
                type="text"
                placeholder="e.g. 100k, 1.5M, 0 = unlimited"
                value={tokenBudget}
                onChange={e => setTokenBudget(e.target.value)}
              />
            </div>
            <div className="flex flex-col gap-1.5">
              <Label htmlFor="key-upstreams">Allowed Upstreams</Label>
              <Input
                id="key-upstreams"
                placeholder="comma-separated, blank = all"
                value={allowedUpstreams}
                onChange={e => setAllowedUpstreams(e.target.value)}
              />
            </div>
            <div className="flex flex-col gap-1.5">
              <Label htmlFor="key-expires">Expires At</Label>
              <Input
                id="key-expires"
                type="datetime-local"
                value={expiresAt}
                onChange={e => setExpiresAt(e.target.value)}
              />
            </div>
            <div className="flex flex-col gap-1.5">
              <Label htmlFor="key-rpm">Rate Limit/min</Label>
              <Input
                id="key-rpm"
                type="number"
                placeholder="0 = unlimited"
                value={rateLimitPerMinute}
                onChange={e => setRateLimitPerMinute(e.target.value)}
              />
            </div>
            <div className="flex flex-col gap-1.5">
              <Label htmlFor="key-rpd">Rate Limit/day</Label>
              <Input
                id="key-rpd"
                type="number"
                placeholder="0 = unlimited"
                value={rateLimitPerDay}
                onChange={e => setRateLimitPerDay(e.target.value)}
              />
            </div>
            <DialogFooter>
              <Button onClick={handleSubmit} disabled={submitting}>
                Create Key
              </Button>
            </DialogFooter>
          </div>
        )}
      </DialogContent>
    </Dialog>
  )
}

import { useState } from 'react'
import { toast } from 'sonner'

import { apiFetch } from '@/lib/api'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'

interface BlockKeyDialogProps {
  keyId: string | null
  keyName: string
  open: boolean
  onOpenChange: (open: boolean) => void
  onBlocked: () => void
}

export default function BlockKeyDialog({
  keyId,
  open,
  onOpenChange,
  onBlocked,
}: BlockKeyDialogProps) {
  const [submitting, setSubmitting] = useState(false)

  async function handleBlock() {
    if (!keyId) return
    setSubmitting(true)
    try {
      await apiFetch(`/api/keys/${keyId}/block`, { method: 'POST' })
      onBlocked()
      onOpenChange(false)
    } catch (err: unknown) {
      const msg = err instanceof Error ? err.message : 'Failed to block key'
      toast.error(msg)
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Block this key?</DialogTitle>
          <DialogDescription>
            Blocked keys receive 401 on every request. You can unblock at any time.
          </DialogDescription>
        </DialogHeader>
        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            Cancel
          </Button>
          <Button variant="destructive" onClick={handleBlock} disabled={submitting}>
            Block Key
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}

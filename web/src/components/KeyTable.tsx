import { useCallback, useEffect, useState } from 'react'
import { Copy, Info, Pencil } from 'lucide-react'

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
import { toast } from 'sonner'

import { apiFetch } from '@/lib/api'
import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import { Skeleton } from '@/components/ui/skeleton'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import CreateKeyDialog from './CreateKeyDialog'
import BlockKeyDialog from './BlockKeyDialog'
import EditKeyDialog from './EditKeyDialog'

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

function maskToken(token: string): string {
  if (token.length <= 10) return '***'
  return token.slice(0, 7) + '***' + token.slice(-3)
}

function getKeyStatus(key: KeyResponse): { label: string; className: string } {
  if (!key.enabled)
    return { label: 'Blocked', className: 'bg-red-500/10 text-red-600 border-red-200' }
  if (key.expires_at && new Date(key.expires_at) < new Date())
    return { label: 'Expired', className: 'bg-gray-500/10 text-gray-500 border-gray-200' }
  if (key.token_budget > 0 && key.usage_total_input + key.usage_total_output >= key.token_budget)
    return { label: 'Out of Budget', className: 'bg-amber-500/10 text-amber-600 border-amber-200' }
  if (key.rate_limit_per_day > 0 && key.day_usage >= key.rate_limit_per_day)
    return { label: 'Rate Limited', className: 'bg-amber-500/10 text-amber-600 border-amber-200' }
  return { label: 'Active', className: 'bg-green-500/10 text-green-600 border-green-200' }
}

export default function KeyTable() {
  const [keys, setKeys] = useState<KeyResponse[]>([])
  const [loading, setLoading] = useState(true)
  const [createOpen, setCreateOpen] = useState(false)
  const [blockTarget, setBlockTarget] = useState<{ id: string; name: string } | null>(null)
  const [editTarget, setEditTarget] = useState<KeyResponse | null>(null)

  const refetchKeys = useCallback(async () => {
    setLoading(true)
    try {
      const data = await apiFetch<KeyResponse[]>('/api/keys')
      setKeys(data)
    } catch {
      toast.error('Failed to load keys')
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => {
    refetchKeys()
  }, [refetchKeys])

  async function handleUnblock(keyId: string) {
    try {
      await apiFetch(`/api/keys/${keyId}/unblock`, { method: 'POST' })
      await refetchKeys()
    } catch (err: unknown) {
      const msg = err instanceof Error ? err.message : 'Failed to unblock key'
      toast.error(msg)
    }
  }

  return (
    <div>
      <div className="flex items-center justify-between mb-4">
        <h2 className="text-xl font-semibold">Access Keys</h2>
        <Button onClick={() => setCreateOpen(true)}>Create Key</Button>
      </div>

      {loading ? (
        <div className="flex flex-col gap-2">
          <Skeleton className="h-8" />
          <Skeleton className="h-8" />
          <Skeleton className="h-8" />
        </div>
      ) : keys.length === 0 ? (
        <div className="py-16 text-center">
          <p className="text-lg font-semibold">No access keys</p>
          <p className="text-sm text-muted-foreground mt-1">
            Create your first key to start proxying requests.
          </p>
        </div>
      ) : (
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead className="text-xs font-normal">Name</TableHead>
              <TableHead className="text-xs font-normal">Key</TableHead>
              <TableHead className="text-xs font-normal">Status</TableHead>
              <TableHead className="text-xs font-normal">Budget</TableHead>
              <TableHead className="text-xs font-normal">Expires</TableHead>
              <TableHead className="text-xs font-normal">Usage</TableHead>
              <TableHead className="text-xs font-normal">Rate/min</TableHead>
              <TableHead className="text-xs font-normal">Rate/day</TableHead>
              <TableHead className="text-xs font-normal">Today Reqs</TableHead>
              <TableHead className="text-xs font-normal">Actions</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {keys.map(key => (
              <TableRow key={key.id}>
                <TableCell className="text-sm">{key.name}</TableCell>
                <TableCell>
                  <div className="flex items-center gap-1">
                    <span className="font-mono text-xs">{maskToken(key.token)}</span>
                    <Button
                      variant="ghost"
                      size="sm"
                      aria-label="Copy token"
                      className="h-5 w-5 p-0"
                      onClick={() => {
                        copyText(key.token)
                        toast.success('Token copied')
                      }}
                    >
                      <Copy className="h-3 w-3" />
                    </Button>
                  </div>
                </TableCell>
                <TableCell>
                  {(() => { const s = getKeyStatus(key); return <Badge className={s.className}>{s.label}</Badge> })()}
                </TableCell>
                <TableCell className="text-sm">
                  {key.token_budget === 0 ? 'Unlimited' : key.token_budget.toLocaleString()}
                </TableCell>
                <TableCell className="text-sm">
                  {key.expires_at ? new Date(key.expires_at).toLocaleDateString() : 'Never'}
                </TableCell>
                <TableCell className="text-sm">
                  {(key.usage_total_input + key.usage_total_output).toLocaleString()} tokens
                </TableCell>
                <TableCell className="text-sm">
                  {key.rate_limit_per_minute === 0 ? 'Unlimited' : key.rate_limit_per_minute}
                </TableCell>
                <TableCell className="text-sm">
                  {key.rate_limit_per_day === 0 ? 'Unlimited' : key.rate_limit_per_day}
                </TableCell>
                <TableCell className="text-sm">{key.day_usage}</TableCell>
                <TableCell>
                  <div className="flex items-center gap-1">
                    <Button
                      variant="ghost"
                      size="sm"
                      aria-label="Edit key"
                      onClick={() => setEditTarget(key)}
                    >
                      <Pencil className="h-4 w-4" />
                    </Button>
                    {key.enabled ? (
                      <Button
                        variant="outline"
                        size="sm"
                        className="text-destructive"
                        onClick={() => setBlockTarget({ id: key.id, name: key.name })}
                      >
                        Block Key
                      </Button>
                    ) : (
                      <Button
                        variant="outline"
                        size="sm"
                        onClick={() => handleUnblock(key.id)}
                      >
                        Unblock Key
                      </Button>
                    )}
                    <Button
                      variant="ghost"
                      size="sm"
                      aria-label="View key details"
                      onClick={() => console.log('detail', key.id)}
                    >
                      <Info className="h-4 w-4" />
                    </Button>
                  </div>
                </TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      )}

      <CreateKeyDialog
        open={createOpen}
        onOpenChange={setCreateOpen}
        onCreated={refetchKeys}
      />
      <BlockKeyDialog
        keyId={blockTarget?.id ?? null}
        keyName={blockTarget?.name ?? ''}
        open={!!blockTarget}
        onOpenChange={(open) => { if (!open) setBlockTarget(null) }}
        onBlocked={refetchKeys}
      />
      <EditKeyDialog
        open={editTarget !== null}
        onOpenChange={(open) => { if (!open) setEditTarget(null) }}
        onUpdated={refetchKeys}
        keyData={editTarget}
      />
    </div>
  )
}

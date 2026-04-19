import { useCallback, useEffect, useState } from 'react'
import { Pencil, RefreshCw } from 'lucide-react'
import { toast } from 'sonner'

import { apiFetch } from '@/lib/api'
import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import { Card, CardContent, CardHeader } from '@/components/ui/card'
import { Skeleton } from '@/components/ui/skeleton'
import { Switch } from '@/components/ui/switch'
import AddUpstreamDialog from './AddUpstreamDialog'
import EditUpstreamDialog, { type UpstreamInfo } from './EditUpstreamDialog'

const PROVIDER_LABELS: Record<string, string> = {
  minimax: 'Minimax', mimo: 'Mimo', kimi: 'Kimi', qwen: 'Qwen', glm: 'GLM', deepseek: 'DeepSeek',
}
function displayName(name: string): string {
  return PROVIDER_LABELS[name.toLowerCase()] ?? name
}

export default function UpstreamStatus() {
  const [upstreams, setUpstreams] = useState<UpstreamInfo[]>([])
  const [loading, setLoading] = useState(true)
  const [editTarget, setEditTarget] = useState<UpstreamInfo | null>(null)
  const [addOpen, setAddOpen] = useState(false)

  const fetchUpstreams = useCallback(async () => {
    setLoading(true)
    try {
      const data = await apiFetch<UpstreamInfo[]>('/api/upstreams')
      setUpstreams(data)
    } catch {
      toast.error('Failed to load upstreams')
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => {
    fetchUpstreams()
  }, [fetchUpstreams])

  function getBadgeProps(enabled: boolean, available: boolean): { label: string; className: string } {
    if (enabled && available) {
      return { label: 'Healthy', className: 'bg-blue-500/10 text-blue-600 border-blue-200' }
    }
    if (enabled && !available) {
      return { label: 'Cooldown', className: 'bg-amber-500/10 text-amber-600 border-amber-200' }
    }
    if (!enabled && available) {
      return { label: 'Disabled', className: 'bg-gray-500/10 text-gray-500 border-gray-200' }
    }
    return { label: 'Unavailable', className: 'bg-gray-500/10 text-gray-500 border-gray-200' }
  }

  function getDotColor(enabled: boolean, available: boolean): string {
    if (enabled && available) return 'bg-blue-500'
    if (enabled && !available) return 'bg-amber-500'
    return 'bg-gray-400'
  }

  async function handleToggle(upstream: UpstreamInfo) {
    const prev = upstreams
    setUpstreams(us =>
      us.map(u => u.id === upstream.id ? { ...u, enabled: !u.enabled } : u)
    )
    try {
      await apiFetch(`/api/upstreams/${upstream.id}/toggle`, { method: 'POST' })
    } catch {
      setUpstreams(prev)
      toast.error('Failed to toggle upstream')
    }
  }

  return (
    <div>
      <div className="flex items-center justify-between mb-4">
        <h2 className="text-xl font-semibold">Upstream Status</h2>
        <div className="flex items-center gap-2">
          <Button variant="outline" size="sm" onClick={() => setAddOpen(true)}>Add Upstream</Button>
          <Button
            variant="ghost"
            size="sm"
            aria-label="Refresh upstream status"
            onClick={fetchUpstreams}
          >
            <RefreshCw className="h-4 w-4" />
          </Button>
        </div>
      </div>

      {loading ? (
        <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
          <Skeleton className="h-24" />
          <Skeleton className="h-24" />
          <Skeleton className="h-24" />
        </div>
      ) : (
        <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
          {upstreams.map(upstream => {
            const badge = getBadgeProps(upstream.enabled, upstream.available)
            const dot = getDotColor(upstream.enabled, upstream.available)
            return (
              <Card key={upstream.id}>
                <CardHeader className="pb-2">
                  <div className="flex items-center justify-between">
                    <span className="text-sm font-normal">{displayName(upstream.name)}</span>
                    <div className="flex items-center gap-1.5">
                      <span className={`h-2 w-2 rounded-full ${dot} inline-block`} />
                      <Badge className={badge.className}>{badge.label}</Badge>
                    </div>
                  </div>
                </CardHeader>
                <CardContent>
                  <div className="flex items-center justify-between">
                    <div className="flex items-center gap-2">
                      <Switch
                        checked={upstream.enabled}
                        onCheckedChange={() => handleToggle(upstream)}
                      />
                      <span className="text-sm text-muted-foreground">
                        {upstream.enabled ? 'Enabled' : 'Disabled'}
                      </span>
                    </div>
                    <Button
                      variant="ghost"
                      size="sm"
                      aria-label="Edit upstream"
                      onClick={() => setEditTarget(upstream)}
                    >
                      <Pencil className="h-4 w-4" />
                    </Button>
                  </div>
                </CardContent>
              </Card>
            )
          })}
        </div>
      )}

      <EditUpstreamDialog
        open={editTarget !== null}
        onOpenChange={(open) => { if (!open) setEditTarget(null) }}
        onUpdated={fetchUpstreams}
        upstream={editTarget}
      />
      <AddUpstreamDialog open={addOpen} onOpenChange={setAddOpen} onCreated={fetchUpstreams} />
    </div>
  )
}

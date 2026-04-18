import { useCallback, useEffect, useState } from 'react'
import { RefreshCw } from 'lucide-react'
import { toast } from 'sonner'

import { apiFetch } from '@/lib/api'
import { Button } from '@/components/ui/button'
import { Skeleton } from '@/components/ui/skeleton'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'

interface UsageRow {
  key_name: string
  total_requests: number
  total_input_tokens: number
  total_output_tokens: number
}

export default function UsagePage() {
  const [rows, setRows] = useState<UsageRow[]>([])
  const [loading, setLoading] = useState(true)

  const fetchUsage = useCallback(async () => {
    setLoading(true)
    try {
      const data = await apiFetch<UsageRow[]>('/api/usage')
      setRows(data)
    } catch {
      toast.error('Failed to load usage data')
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => {
    fetchUsage()
  }, [fetchUsage])

  return (
    <div>
      <div className="flex items-center justify-between mb-4">
        <h2 className="text-xl font-semibold">Usage Statistics</h2>
        <Button variant="ghost" size="sm" aria-label="Refresh usage" onClick={fetchUsage}>
          <RefreshCw className="h-4 w-4" />
        </Button>
      </div>

      {loading ? (
        <div className="flex flex-col gap-2">
          <Skeleton className="h-8" />
          <Skeleton className="h-8" />
          <Skeleton className="h-8" />
        </div>
      ) : rows.length === 0 ? (
        <div className="py-16 text-center">
          <p className="text-lg font-semibold">No usage data</p>
          <p className="text-sm text-muted-foreground mt-1">No requests have been recorded yet.</p>
        </div>
      ) : (
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead className="text-sm">Key Name</TableHead>
              <TableHead className="text-sm">Requests</TableHead>
              <TableHead className="text-sm">Input Tokens</TableHead>
              <TableHead className="text-sm">Output Tokens</TableHead>
              <TableHead className="text-sm font-semibold">Total Tokens</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {rows.map((row) => (
              <TableRow key={row.key_name}>
                <TableCell className="text-sm">{row.key_name}</TableCell>
                <TableCell className="text-sm">{row.total_requests.toLocaleString()}</TableCell>
                <TableCell className="text-sm">{row.total_input_tokens.toLocaleString()}</TableCell>
                <TableCell className="text-sm">{row.total_output_tokens.toLocaleString()}</TableCell>
                <TableCell className="text-sm font-semibold">
                  {(row.total_input_tokens + row.total_output_tokens).toLocaleString()}
                </TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      )}
    </div>
  )
}

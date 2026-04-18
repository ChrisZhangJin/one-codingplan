import { useState } from 'react'
import { Separator } from '@/components/ui/separator'
import { Button } from '@/components/ui/button'
import UpstreamStatus from '@/components/UpstreamStatus'
import KeyTable from '@/components/KeyTable'
import UsagePage from '@/components/UsagePage'

export default function DashboardPage() {
  const [activeTab, setActiveTab] = useState<'dashboard' | 'usage'>('dashboard')

  return (
    <div className="max-w-5xl mx-auto p-8">
      <div className="flex gap-1 border-b border-border mb-6">
        <Button
          variant={activeTab === 'dashboard' ? 'secondary' : 'ghost'}
          className="text-sm font-semibold"
          onClick={() => setActiveTab('dashboard')}
        >
          Dashboard
        </Button>
        <Button
          variant={activeTab === 'usage' ? 'secondary' : 'ghost'}
          className="text-sm font-semibold"
          onClick={() => setActiveTab('usage')}
        >
          Usage
        </Button>
      </div>

      {activeTab === 'dashboard' ? (
        <>
          <UpstreamStatus />
          <Separator className="my-8" />
          <KeyTable />
        </>
      ) : (
        <UsagePage />
      )}
    </div>
  )
}

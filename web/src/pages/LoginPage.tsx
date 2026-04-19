import { useState } from 'react'
import { Card, CardContent, CardHeader } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Button } from '@/components/ui/button'
import { setToken } from '@/lib/auth'

interface LoginPageProps {
  onLogin: () => void
}

export default function LoginPage({ onLogin }: LoginPageProps) {
  const [key, setKey] = useState('')
  const [error, setError] = useState('')
  const [loading, setLoading] = useState(false)

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault()
    setError('')
    setLoading(true)
    try {
      const res = await fetch('/api/keys', {
        headers: { 'Authorization': `Bearer ${key}` },
      })
      if (res.ok) {
        setToken(key)
        onLogin()
      } else {
        setError('Invalid admin key. Check your admin_key config value.')
      }
    } catch {
      setError('Could not reach the server. Check that ocp is running.')
    } finally {
      setLoading(false)
    }
  }

  return (
    <div className="min-h-screen flex items-center justify-center">
      <Card className="w-[360px]">
        <CardHeader>
          <p className="text-[28px] font-semibold leading-[1.2]">one-codingplan</p>
          <p className="text-sm text-muted-foreground">Enter your admin key to continue</p>
        </CardHeader>
        <CardContent>
          <form onSubmit={handleSubmit} className="flex flex-col gap-4">
            <div className="flex flex-col gap-2">
              <Label htmlFor="admin-key">Admin Key</Label>
              <Input
                id="admin-key"
                type="password"
                value={key}
                onChange={(e) => setKey(e.target.value)}
                required
              />
            </div>
            {error && (
              <p className="text-destructive text-sm">{error}</p>
            )}
            <Button type="submit" className="w-full" disabled={loading}>
              Sign in
            </Button>
          </form>
        </CardContent>
      </Card>
    </div>
  )
}

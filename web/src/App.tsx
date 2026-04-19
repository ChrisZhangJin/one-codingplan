import { useState } from 'react'
import { getToken } from './lib/auth'
import LoginPage from './pages/LoginPage'
import DashboardPage from './pages/DashboardPage'
import { Toaster } from '@/components/ui/sonner'

function App() {
  const [loggedIn, setLoggedIn] = useState(!!getToken())

  if (!loggedIn) {
    return (
      <>
        <LoginPage onLogin={() => setLoggedIn(true)} />
        <Toaster />
      </>
    )
  }

  return (
    <>
      <DashboardPage />
      <Toaster />
    </>
  )
}

export default App

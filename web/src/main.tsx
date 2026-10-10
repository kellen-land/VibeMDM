import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { BrowserRouter } from 'react-router-dom'
import './index.css'
import { App } from './App'
import { AuthProvider, meKey } from './auth/AuthContext'
import { ThemeProvider } from './theme/ThemeContext'
import { ToastProvider } from './components/ui/Toast'
import { ApiError, setPasswordChangeHandler } from './api/client'

const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      staleTime: 10_000,
      refetchOnWindowFocus: true,
      retry: (count, err) => {
        if (err instanceof ApiError && err.status >= 400 && err.status < 500) return false
        return count < 2
      },
    },
    mutations: { retry: false },
  },
})

// Any call answering 403 "password change required" means this session must choose a new
// password: refetch me so RequireAuth swaps the app shell for that screen.
setPasswordChangeHandler(() => void queryClient.invalidateQueries({ queryKey: meKey }))

createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <QueryClientProvider client={queryClient}>
      <BrowserRouter>
        <ThemeProvider>
          <ToastProvider>
            <AuthProvider>
              <App />
            </AuthProvider>
          </ToastProvider>
        </ThemeProvider>
      </BrowserRouter>
    </QueryClientProvider>
  </StrictMode>,
)

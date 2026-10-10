import { createContext, useCallback, useContext, useMemo, type ReactNode } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { api, ApiError } from '../api/client'
import type { AdminUser, PublicStatus } from '../api/types'

interface AuthState {
  user: AdminUser | null
  loading: boolean
  isAdmin: boolean
  status: PublicStatus | undefined
  login: (username: string, password: string) => Promise<AdminUser>
  logout: () => Promise<void>
  refresh: () => Promise<void>
}

const AuthContext = createContext<AuthState | null>(null)

export const meKey = ['auth', 'me'] as const
export const statusKey = ['status'] as const

export function AuthProvider({ children }: { children: ReactNode }) {
  const qc = useQueryClient()
  const me = useQuery({
    queryKey: meKey,
    queryFn: async () => {
      try {
        return await api.get<AdminUser>('/api/v1/auth/me')
      } catch (e) {
        if (e instanceof ApiError && e.status === 401) return null
        throw e
      }
    },
    staleTime: 5 * 60_000,
    retry: false,
  })
  const status = useQuery({
    queryKey: statusKey,
    queryFn: () => api.get<PublicStatus>('/api/v1/status'),
    staleTime: 10 * 60_000,
  })

  const login = useCallback(
    async (username: string, password: string) => {
      const u = await api.post<AdminUser>('/api/v1/auth/login', { username, password })
      qc.setQueryData(meKey, u)
      await qc.invalidateQueries({ queryKey: statusKey })
      return u
    },
    [qc],
  )
  const logout = useCallback(async () => {
    try {
      await api.post('/api/v1/auth/logout')
    } finally {
      qc.clear()
      qc.setQueryData(meKey, null)
    }
  }, [qc])
  const refresh = useCallback(async () => {
    await qc.invalidateQueries({ queryKey: meKey })
  }, [qc])

  const value = useMemo<AuthState>(
    () => ({
      user: me.data ?? null,
      loading: me.isPending,
      isAdmin: me.data?.role === 'admin',
      status: status.data,
      login,
      logout,
      refresh,
    }),
    [me.data, me.isPending, status.data, login, logout, refresh],
  )
  return <AuthContext.Provider value={value}>{children}</AuthContext.Provider>
}

export function useAuth(): AuthState {
  const ctx = useContext(AuthContext)
  if (!ctx) throw new Error('useAuth must be used within AuthProvider')
  return ctx
}

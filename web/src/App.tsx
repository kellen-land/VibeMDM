import { Navigate, Route, Routes, useLocation } from 'react-router-dom'
import { useAuth } from './auth/AuthContext'
import { AppShell } from './components/layout/AppShell'
import { LoadingBlock } from './components/ui/Spinner'
import { LoginPage } from './pages/LoginPage'
import { DashboardPage } from './pages/DashboardPage'
import { ComputersPage } from './pages/computers/ComputersPage'
import { ComputerDetailPage } from './pages/computers/ComputerDetailPage'
import { GroupsPage } from './pages/GroupsPage'
import { GroupEditorPage } from './pages/GroupEditorPage'
import { PoliciesPage } from './pages/PoliciesPage'
import { PolicyEditorPage } from './pages/PolicyEditorPage'
import { ScriptsPage } from './pages/ScriptsPage'
import { ScriptEditorPage } from './pages/ScriptEditorPage'
import { ProfilesPage } from './pages/ProfilesPage'
import { ProfileEditorPage } from './pages/ProfileEditorPage'
import { PackagesPage } from './pages/PackagesPage'
import { ExtensionAttributesPage } from './pages/ExtensionAttributesPage'
import { ExtensionAttributeEditorPage } from './pages/ExtensionAttributeEditorPage'
import { ActivityPage } from './pages/ActivityPage'
import { EnrollmentPage } from './pages/EnrollmentPage'
import { SettingsPage } from './pages/settings/SettingsPage'
import { RestrictedSoftwarePage } from './pages/RestrictedSoftwarePage'
import { RestrictedSoftwareEditorPage } from './pages/RestrictedSoftwareEditorPage'
import { SearchesPage } from './pages/SearchesPage'
import { SearchEditorPage } from './pages/SearchEditorPage'
import { AppsPage } from './pages/AppsPage'
import { AppEditorPage } from './pages/AppEditorPage'
import { NotFoundPage } from './pages/NotFoundPage'
import { CommandInspectorPage } from './pages/CommandInspectorPage'
import { FileVaultPage } from './pages/FileVaultPage'
import { LogsPage } from './pages/LogsPage'
import { CustomLabelsPage } from './pages/CustomLabelsPage'
import { CustomLabelEditorPage } from './pages/CustomLabelEditorPage'

function RequireAuth({ children }: { children: React.ReactNode }) {
  const { user, loading } = useAuth()
  const loc = useLocation()
  if (loading) return <LoadingBlock className="h-screen" />
  if (!user) return <Navigate to={`/login?next=${encodeURIComponent(loc.pathname + loc.search)}`} replace />
  return <>{children}</>
}

export function App() {
  return (
    <Routes>
      <Route path="/login" element={<LoginPage />} />
      <Route
        element={
          <RequireAuth>
            <AppShell />
          </RequireAuth>
        }
      >
        <Route path="/" element={<DashboardPage />} />
        <Route path="/computers" element={<ComputersPage />} />
        <Route path="/computers/:id" element={<ComputerDetailPage />} />
        <Route path="/computers/:id/:tab" element={<ComputerDetailPage />} />
        <Route path="/groups" element={<GroupsPage />} />
        <Route path="/groups/new" element={<GroupEditorPage />} />
        <Route path="/groups/:id" element={<GroupEditorPage />} />
        <Route path="/searches" element={<SearchesPage />} />
        <Route path="/searches/new" element={<SearchEditorPage />} />
        <Route path="/searches/:id" element={<SearchEditorPage />} />
        <Route path="/policies" element={<PoliciesPage />} />
        <Route path="/policies/new" element={<PolicyEditorPage />} />
        <Route path="/policies/:id" element={<PolicyEditorPage />} />
        <Route path="/scripts" element={<ScriptsPage />} />
        <Route path="/scripts/new" element={<ScriptEditorPage />} />
        <Route path="/scripts/:id" element={<ScriptEditorPage />} />
        <Route path="/profiles" element={<ProfilesPage />} />
        <Route path="/profiles/new" element={<ProfileEditorPage />} />
        <Route path="/profiles/:id" element={<ProfileEditorPage />} />
        <Route path="/packages" element={<PackagesPage />} />
        <Route path="/extension-attributes" element={<ExtensionAttributesPage />} />
        <Route path="/extension-attributes/new" element={<ExtensionAttributeEditorPage />} />
        <Route path="/extension-attributes/:id" element={<ExtensionAttributeEditorPage />} />
        <Route path="/restricted-software" element={<RestrictedSoftwarePage />} />
        <Route path="/restricted-software/events" element={<RestrictedSoftwarePage />} />
        <Route path="/restricted-software/new" element={<RestrictedSoftwareEditorPage />} />
        <Route path="/restricted-software/:id" element={<RestrictedSoftwareEditorPage />} />
        <Route path="/apps" element={<AppsPage />} />
        <Route path="/apps/custom-labels" element={<CustomLabelsPage />} />
        <Route path="/apps/custom-labels/:id" element={<CustomLabelEditorPage />} />
        <Route path="/apps/new" element={<AppEditorPage />} />
        <Route path="/apps/:id" element={<AppEditorPage />} />
        <Route path="/activity" element={<ActivityPage />} />
        <Route path="/activity/:tab" element={<ActivityPage />} />
        <Route path="/logs" element={<LogsPage />} />
        <Route path="/logs/:tab" element={<LogsPage />} />
        <Route path="/commands/:uuid" element={<CommandInspectorPage />} />
        <Route path="/filevault" element={<FileVaultPage />} />
        <Route path="/enrollment" element={<EnrollmentPage />} />
        <Route path="/settings" element={<SettingsPage />} />
        <Route path="/settings/:tab" element={<SettingsPage />} />
        <Route path="*" element={<NotFoundPage />} />
      </Route>
    </Routes>
  )
}

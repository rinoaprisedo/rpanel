import { Navigate, useSearchParams } from 'react-router-dom'
import Tabs from '../../components/ui/Tabs'
import PanelSsl from './PanelSsl'
import { useEffect, useState, useMemo } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import toast from 'react-hot-toast'
import Input from '../../components/ui/Input'
import Button from '../../components/ui/Button'
import Skeleton from '../../components/ui/Skeleton'
import FileUpload from '../../components/ui/FileUpload'
import { Menubar, MenubarLabel } from '../../components/ui/Menubar'
import { getSettings, updateSettings, uploadSetting } from '../../api/settings'
import { fileURL } from '../../utils/url'
import usePageActions from '../../hooks/usePageActions'
import useSettingsStore from '../../store/settingsStore'

const tabs = ['General', 'Appearance', 'Panel SSL', 'Maintenance']

function Settings() {
  const [params] = useSearchParams()
  const [tab, setTab] = useState({ 'panel-ssl': 'Panel SSL' }[params.get('tab')] || 'General')
  const [appName, setAppName] = useState('')
  const [maintenance, setMaintenance] = useState(false)
  const [logoPreview, setLogoPreview] = useState(null)
  const [faviconPreview, setFaviconPreview] = useState(null)
  const [primaryColor, setPrimaryColorInput] = useState('#35494d')
  const queryClient = useQueryClient()
  const applyPrimaryColorLive = useSettingsStore((s) => s.setPrimaryColor)
  const storedPrimaryColor = useSettingsStore((s) => s.primaryColor)

  const { data, isLoading } = useQuery({ queryKey: ['settings'], queryFn: getSettings })

  useEffect(() => {
    if (data?.data) {
      // GET /settings returns a { key: value } map directly, not an array.
      const map = data.data
      setAppName(map.app_name || '')
      setMaintenance(map.maintenance_mode === 'true' || map.maintenance_mode === true)
      setLogoPreview(fileURL(map.app_logo))
      setFaviconPreview(fileURL(map.app_favicon))
      setPrimaryColorInput(map.primary_color || storedPrimaryColor)
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [data])

  const handlePrimaryColorChange = (hex) => {
    setPrimaryColorInput(hex)
    applyPrimaryColorLive(hex) // live-preview across the whole app before saving
  }

  const saveMutation = useMutation({
    mutationFn: updateSettings,
    onSuccess: () => {
      toast.success('Settings saved')
      queryClient.invalidateQueries({ queryKey: ['settings'] })
    },
    onError: (err) => toast.error(err.response?.data?.message || 'Save failed'),
  })

  const uploadMutation = useMutation({
    mutationFn: uploadSetting,
    onSuccess: () => {
      toast.success('File uploaded')
      queryClient.invalidateQueries({ queryKey: ['settings'] })
    },
    onError: (err) => toast.error(err.response?.data?.message || 'Upload failed'),
  })

  const handleFileChange = (key) => (file) => {
    const formData = new FormData()
    formData.append('file', file)
    formData.append('target', key)
    uploadMutation.mutate(formData)
    const reader = new FileReader()
    reader.onload = () => (key === 'app_logo' ? setLogoPreview(reader.result) : setFaviconPreview(reader.result))
    reader.readAsDataURL(file)
  }

  usePageActions(
    useMemo(
      () => (
        <Menubar>
          <MenubarLabel>Settings</MenubarLabel>
        </Menubar>
      ),
      [],
    ),
  )

  if (params.get('tab') === 'rpanel-update') return <Navigate to="/updates?tab=rpanel" replace /> // moved to the Updates page
  if (isLoading) return <Skeleton className="h-96 w-full" />

  return (
    <div className="space-y-4">
      <Tabs tabs={tabs} value={tab} onChange={setTab} />

      <div className="rounded-lg border border-surface-border bg-surface-card p-6">
        {tab === 'General' && (
          <div className="max-w-md space-y-4">
            <Input label="App Name" value={appName} onChange={(e) => setAppName(e.target.value)} />
            <Button onClick={() => saveMutation.mutate({ app_name: appName })} disabled={saveMutation.isPending}>
              Save
            </Button>
          </div>
        )}

        {tab === 'Appearance' && (
          <div className="max-w-md space-y-6">
            <FileUpload
              label="Logo"
              hint="PNG, JPEG or WebP"
              accept="image/png,image/jpeg,image/webp"
              preview={logoPreview}
              onFileSelect={handleFileChange('app_logo')}
              previewClassName="h-16 w-16"
            />
            <FileUpload
              label="Favicon"
              hint="PNG, JPEG or WebP"
              accept="image/png,image/jpeg,image/webp"
              preview={faviconPreview}
              onFileSelect={handleFileChange('app_favicon')}
              previewClassName="h-12 w-12"
            />
            <div>
              <p className="mb-2 text-sm font-medium text-text-primary">Base Color</p>
              <div className="flex items-center gap-3">
                <input
                  type="color"
                  value={primaryColor}
                  onChange={(e) => handlePrimaryColorChange(e.target.value)}
                  className="h-10 w-14 cursor-pointer rounded-md border border-surface-border bg-transparent p-1"
                />
                <Input
                  value={primaryColor}
                  onChange={(e) => handlePrimaryColorChange(e.target.value)}
                  className="w-32 uppercase"
                  maxLength={7}
                />
              </div>
              <p className="mt-1 text-xs text-text-secondary">Applied instantly as a preview; click Save to persist.</p>
              <Button
                className="mt-3"
                onClick={() => saveMutation.mutate({ primary_color: primaryColor })}
                disabled={saveMutation.isPending}
              >
                Save Color
              </Button>
            </div>
          </div>
        )}

        {tab === 'Panel SSL' && <PanelSsl />}

        {tab === 'Maintenance' && (
          <div className="max-w-md space-y-4">
            <label className="flex items-center gap-3">
              <input
                type="checkbox"
                checked={maintenance}
                onChange={(e) => setMaintenance(e.target.checked)}
                className="h-4 w-4 accent-primary"
              />
              <span className="text-sm text-text-primary">Enable maintenance mode</span>
            </label>
            <Button
              onClick={() => saveMutation.mutate({ maintenance_mode: maintenance })}
              disabled={saveMutation.isPending}
            >
              Save
            </Button>
          </div>
        )}
      </div>
    </div>
  )
}

export default Settings

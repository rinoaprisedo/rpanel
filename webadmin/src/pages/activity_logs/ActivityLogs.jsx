import { useState, useMemo } from 'react'
import { useQuery } from '@tanstack/react-query'
import { format } from 'date-fns'
import { Filter } from 'lucide-react'
import Table from '../../components/ui/Table'
import Badge from '../../components/ui/Badge'
import Button from '../../components/ui/Button'
import Input from '../../components/ui/Input'
import Select from '../../components/ui/Select'
import { Menubar, MenubarLabel, MenubarSeparator, MenubarMenu } from '../../components/ui/Menubar'
import ActivityLogDetailModal from './ActivityLogDetailModal'
import { getActivityLogs } from '../../api/activityLogs'
import usePageActions from '../../hooks/usePageActions'

const actionVariant = { create: 'success', update: 'primary', delete: 'danger', login: 'success', logout: 'neutral', view: 'neutral' }

function ActivityLogs() {
  const [page, setPage] = useState(1)
  const [limit, setLimit] = useState(20)
  const [search, setSearch] = useState('')
  const [module, setModule] = useState('')
  const [action, setAction] = useState('')
  const [user, setUser] = useState('')
  const [ip, setIp] = useState('')
  const [dateFrom, setDateFrom] = useState('')
  const [dateTo, setDateTo] = useState('')
  const [draftModule, setDraftModule] = useState('')
  const [draftAction, setDraftAction] = useState('')
  const [draftUser, setDraftUser] = useState('')
  const [draftIp, setDraftIp] = useState('')
  const [draftDateFrom, setDraftDateFrom] = useState('')
  const [draftDateTo, setDraftDateTo] = useState('')
  const [selected, setSelected] = useState(null)

  const { data, isLoading } = useQuery({
    queryKey: ['activity-logs', { page, limit, search, module, action, user, ip, dateFrom, dateTo }],
    queryFn: () =>
      getActivityLogs({
        page,
        limit,
        search,
        module: module || undefined,
        action: action || undefined,
        user: user || undefined,
        ip: ip || undefined,
        date_from: dateFrom || undefined,
        date_to: dateTo || undefined,
      }),
  })

  const columns = [
    {
      key: 'created_at',
      label: 'Time',
      render: (row) => (row.created_at ? format(new Date(row.created_at), 'PP p') : '-'),
    },
    { key: 'user', label: 'User', render: (row) => row.user?.name || 'System' },
    {
      key: 'action',
      label: 'Action',
      render: (row) => <Badge variant={actionVariant[row.action] || 'neutral'}>{row.action}</Badge>,
    },
    { key: 'module', label: 'Module' },
    { key: 'record_id', label: 'Record ID', render: (row) => row.record_id || '-' },
    { key: 'ip_address', label: 'IP' },
  ]

  usePageActions(
    useMemo(
      () => (
        <Menubar>
          <MenubarLabel>Activity Logs</MenubarLabel>
          <MenubarSeparator />
          <MenubarMenu
            id="filter"
            label="Filter"
            icon={Filter}
            align="sheet"
            active={!!(action || module || user || ip || dateFrom || dateTo)}
          >
            {(close) => (
              <div className="w-full p-3 sm:w-[420px]">
                <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
                  <Select label="Action" value={draftAction} onChange={(e) => setDraftAction(e.target.value)}>
                    <option value="">All</option>
                    {['create', 'update', 'delete', 'login', 'logout', 'view'].map((a) => (
                      <option key={a} value={a}>
                        {a}
                      </option>
                    ))}
                  </Select>
                  <Input
                    label="Module"
                    value={draftModule}
                    onChange={(e) => setDraftModule(e.target.value)}
                    placeholder="e.g. users"
                  />
                  <Input
                    label="User"
                    value={draftUser}
                    onChange={(e) => setDraftUser(e.target.value)}
                    placeholder="Name or email"
                  />
                  <Input
                    label="IP Address"
                    value={draftIp}
                    onChange={(e) => setDraftIp(e.target.value)}
                    placeholder="e.g. 127.0.0.1"
                  />
                  <Input
                    label="Date From"
                    type="date"
                    value={draftDateFrom}
                    onChange={(e) => setDraftDateFrom(e.target.value)}
                  />
                  <Input
                    label="Date To"
                    type="date"
                    value={draftDateTo}
                    onChange={(e) => setDraftDateTo(e.target.value)}
                  />
                </div>
                <div className="mt-3 flex items-center justify-end gap-2 border-t border-surface-border pt-3">
                  <Button
                    variant="secondary"
                    className="h-[32px] px-3"
                    onClick={() => {
                      setDraftAction('')
                      setDraftModule('')
                      setDraftUser('')
                      setDraftIp('')
                      setDraftDateFrom('')
                      setDraftDateTo('')
                      setAction('')
                      setModule('')
                      setUser('')
                      setIp('')
                      setDateFrom('')
                      setDateTo('')
                      setPage(1)
                      close()
                    }}
                  >
                    Reset
                  </Button>
                  <Button
                    className="h-[32px] px-3"
                    onClick={() => {
                      setAction(draftAction)
                      setModule(draftModule)
                      setUser(draftUser)
                      setIp(draftIp)
                      setDateFrom(draftDateFrom)
                      setDateTo(draftDateTo)
                      setPage(1)
                      close()
                    }}
                  >
                    Apply
                  </Button>
                </div>
              </div>
            )}
          </MenubarMenu>
        </Menubar>
      ),
      [action, module, user, ip, dateFrom, dateTo, draftAction, draftModule, draftUser, draftIp, draftDateFrom, draftDateTo],
    ),
  )

  return (
    <div className="space-y-4">
      <Table
        columns={columns}
        data={data?.data || []}
        loading={isLoading}
        search={search}
        onSearchChange={(v) => {
          setSearch(v)
          setPage(1)
        }}
        page={page}
        limit={limit}
        total={data?.meta?.total || 0}
        onPageChange={setPage}
        onLimitChange={(l) => {
          setLimit(l)
          setPage(1)
        }}
        onRowClick={(row) => setSelected(row)}
      />

      <ActivityLogDetailModal open={!!selected} onClose={() => setSelected(null)} log={selected} />
    </div>
  )
}

export default ActivityLogs

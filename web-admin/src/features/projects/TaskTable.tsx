import { useMemo, useState } from 'react'
import { App as AntdApp, Button, Popconfirm, Select, Space, Table, Tag } from 'antd'
import { PlusOutlined } from '@ant-design/icons'
import type { ColumnsType } from 'antd/es/table'
import { useTranslation } from 'react-i18next'
import type { InspectionTask, TaskStatus, User } from '@/shared/api/types'
import { EmptyState } from '@/shared/ui/EmptyState'
import { useUsers } from '@/features/rbac/api'
import { useDeleteTask, useTasks } from './api'
import { TaskFormModal } from './TaskFormModal'
import { TASK_STATUS_META, TASK_STATUS_ORDER } from './status'

interface TaskTableProps {
  projectId: number
}

/** 项目下的巡查任务表（§P3）：状态用冻结字面量；指派复用 rbac 用户。 */
export function TaskTable({ projectId }: TaskTableProps) {
  const { t } = useTranslation()
  const { message } = AntdApp.useApp()
  const [status, setStatus] = useState<TaskStatus | undefined>(undefined)
  const [assigneeId, setAssigneeId] = useState<number | undefined>(undefined)
  const [formOpen, setFormOpen] = useState(false)
  const [editing, setEditing] = useState<InspectionTask | null>(null)

  const { data, isLoading } = useTasks({
    project_id: projectId,
    status,
    assignee_id: assigneeId,
  })
  const deleteTask = useDeleteTask()
  const rows = useMemo(() => data?.items ?? [], [data])

  const { data: usersData } = useUsers({ page: 1, page_size: 100 })
  const userById = useMemo(() => {
    const map = new Map<number, User>()
    for (const u of usersData?.items ?? []) map.set(u.id, u)
    return map
  }, [usersData])
  const assigneeOptions = (usersData?.items ?? []).map((u) => ({
    value: u.id,
    label: u.display_name || u.username,
  }))

  const openCreate = (): void => {
    setEditing(null)
    setFormOpen(true)
  }
  const openEdit = (row: InspectionTask): void => {
    setEditing(row)
    setFormOpen(true)
  }

  const handleDelete = async (row: InspectionTask): Promise<void> => {
    await deleteTask.mutateAsync(row.id)
    message.success(t('proj.taskDeleted'))
  }

  const columns: ColumnsType<InspectionTask> = [
    { title: t('proj.taskTitle'), dataIndex: 'title', key: 'title' },
    {
      title: t('proj.taskStatus'),
      dataIndex: 'status',
      key: 'status',
      width: 110,
      render: (value: TaskStatus) => {
        const meta = TASK_STATUS_META[value]
        return <Tag color={meta.color}>{meta.label}</Tag>
      },
    },
    {
      title: t('proj.assignee'),
      dataIndex: 'assignee_id',
      key: 'assignee_id',
      width: 140,
      render: (value: number | null | undefined) => {
        if (value === null || value === undefined) {
          return <span style={{ color: 'var(--color-ink-muted)' }}>{t('proj.unassigned')}</span>
        }
        const u = userById.get(value)
        return <span>{u ? u.display_name || u.username : `#${value}`}</span>
      },
    },
    {
      title: t('proj.actions'),
      key: 'actions',
      width: 150,
      render: (_value, row) => (
        <Space size="small">
          <Button type="link" size="small" onClick={() => openEdit(row)}>
            {t('common.edit')}
          </Button>
          <Popconfirm
            title={t('proj.deleteTaskConfirm')}
            okText={t('common.ok')}
            cancelText={t('common.cancel')}
            onConfirm={() => handleDelete(row)}
          >
            <Button type="link" size="small" danger>
              {t('proj.delete')}
            </Button>
          </Popconfirm>
        </Space>
      ),
    },
  ]

  return (
    <>
      <Space size="middle" wrap style={{ marginBlockEnd: 'var(--space-4)' }}>
        <Select<TaskStatus>
          allowClear
          placeholder={t('proj.taskStatus')}
          style={{ minWidth: 140 }}
          value={status}
          onChange={(value) => setStatus(value)}
          options={TASK_STATUS_ORDER.map((s) => ({ value: s, label: TASK_STATUS_META[s].label }))}
        />
        <Select<number>
          allowClear
          placeholder={t('proj.assignee')}
          style={{ minWidth: 160 }}
          value={assigneeId}
          onChange={(value) => setAssigneeId(value)}
          options={assigneeOptions}
        />
        <Button type="primary" icon={<PlusOutlined />} onClick={openCreate}>
          {t('proj.newTask')}
        </Button>
      </Space>
      <Table<InspectionTask>
        rowKey="id"
        size="middle"
        loading={isLoading}
        columns={columns}
        dataSource={rows}
        pagination={false}
        locale={{ emptyText: <EmptyState description={t('proj.tasksEmpty')} /> }}
      />
      <TaskFormModal
        open={formOpen}
        projectId={projectId}
        editing={editing}
        onClose={() => setFormOpen(false)}
      />
    </>
  )
}

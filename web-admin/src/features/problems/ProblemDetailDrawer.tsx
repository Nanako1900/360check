import { useState } from 'react'
import {
  App as AntdApp,
  Button,
  Descriptions,
  Divider,
  Drawer,
  Popconfirm,
  Space,
  Spin,
  Typography,
} from 'antd'
import { EyeOutlined, PlusOutlined } from '@ant-design/icons'
import { useTranslation } from 'react-i18next'
import { ApiError } from '@/shared/api/apiError'
import { DictTag } from '@/shared/ui/DictTag'
import { EmptyState } from '@/shared/ui/EmptyState'
import { fmt } from '@/shared/time/dayjs'
import { useAuthStore } from '@/shared/auth/authStore'
import { PanoramaModal } from '@/features/panorama/PanoramaModal'
import type { PanoramaContext } from '@/features/panorama/PanoramaModal'
import { useProject } from '@/features/projects/api'
import { useUsers } from '@/features/rbac/api'
import { useDeleteProblem, useProblem } from './api'
import { ProblemEditForm } from './ProblemEditForm'
import { ProblemProcessingLog } from './ProblemProcessingLog'
import { ProblemPointMap } from './ProblemPointMap'
import { AddProcessingLogModal } from './AddProcessingLogModal'

interface ProblemDetailDrawerProps {
  /** 要查看的问题 id；null 关闭抽屉。 */
  problemId: number | null
  onClose: () => void
}

const { Text } = Typography

/**
 * ProblemDetailDrawer（§P6）：`GET /problems/{id}` 详情抽屉。
 *
 * 组合：看全景（PanoramaModal via cover_media_id）+ 地图点位（ProblemPointMap）+ 描述/备注 +
 * 关联巡查/项目 + 内嵌 ProblemEditForm（改分类/状态/备注）+ ProblemProcessingLog（审计时间线）
 * + AddProcessingLogModal（COMMENT/REASSIGN）。删除走软删（确认框）。
 *
 * 前端权限仅 UI 门控；隐藏的控件对应请求仍可能 FORBIDDEN（§4.6），由各表单/操作处理。
 */
export function ProblemDetailDrawer({ problemId, onClose }: ProblemDetailDrawerProps) {
  const { t } = useTranslation()
  const { message } = AntdApp.useApp()
  const open = problemId !== null
  const id = problemId ?? 0
  const { data: problem, isLoading, isError } = useProblem(id, open)
  const { data: project } = useProject(problem?.project_id ?? 0, Boolean(problem))
  const { data: usersData } = useUsers({ page: 1, page_size: 200 })
  const inspector = usersData?.items.find((u) => u.id === problem?.inspector_id)

  const canUpdate = useAuthStore((s) => s.hasPermission('problem:update'))
  const canDelete = useAuthStore((s) => s.hasPermission('problem:delete'))
  const canLog = useAuthStore((s) => s.hasPermission('problem_log:write'))

  const deleteProblem = useDeleteProblem()
  const [panoOpen, setPanoOpen] = useState(false)
  const [logOpen, setLogOpen] = useState(false)

  const panoContext: PanoramaContext = {
    projectName: project?.name ?? null,
    inspectorName: inspector ? inspector.display_name || inspector.username : null,
    problemTypeItemId: problem?.type_item_id ?? null,
    problemStatusItemId: problem?.status_item_id ?? null,
    problemTitle: problem?.title ?? null,
  }

  const handleDelete = async (): Promise<void> => {
    try {
      await deleteProblem.mutateAsync(id)
      message.success(t('problem.deleted'))
      onClose()
    } catch (err) {
      if (err instanceof ApiError) {
        if (err.code === 'FORBIDDEN') message.error(t('problem.forbidden'))
        else if (err.code === 'CONFLICT') message.error(t('problem.conflict'))
        else message.error(err.message)
      }
    }
  }

  return (
    <Drawer
      open={open}
      onClose={onClose}
      width={Math.min(720, typeof window !== 'undefined' ? window.innerWidth : 720)}
      title={t('problem.detailTitle')}
      destroyOnHidden
      extra={
        canDelete && problem ? (
          <Popconfirm
            title={t('problem.deleteConfirm')}
            okText={t('common.ok')}
            cancelText={t('common.cancel')}
            onConfirm={handleDelete}
          >
            <Button danger size="small">
              {t('problem.delete')}
            </Button>
          </Popconfirm>
        ) : null
      }
    >
      {isLoading ? (
        <div style={{ display: 'grid', placeItems: 'center', minHeight: 240 }}>
          <Spin />
        </div>
      ) : isError || !problem ? (
        <EmptyState description={t('problem.notFound')} />
      ) : (
        <Space direction="vertical" size="large" style={{ width: '100%' }}>
          <Space wrap>
            <Button
              type="primary"
              icon={<EyeOutlined />}
              disabled={problem.cover_media_id == null}
              onClick={() => setPanoOpen(true)}
            >
              {t('problem.viewPanorama')}
            </Button>
          </Space>

          <ProblemPointMap geom={problem.geom} />

          <Descriptions column={1} size="small" colon={false}>
            <Descriptions.Item label={t('problem.status')}>
              <DictTag code="problem_status" itemId={problem.status_item_id} />
            </Descriptions.Item>
            <Descriptions.Item label={t('problem.type')}>
              <DictTag code="problem_type" itemId={problem.type_item_id} />
            </Descriptions.Item>
            <Descriptions.Item label={t('problem.category')}>
              <DictTag code="problem_category" itemId={problem.category_item_id} />
            </Descriptions.Item>
            <Descriptions.Item label={t('problem.project')}>
              <Text>{project?.name ?? `#${problem.project_id}`}</Text>
            </Descriptions.Item>
            <Descriptions.Item label={t('problem.inspection')}>
              {problem.inspection_id != null ? (
                <Text>#{problem.inspection_id}</Text>
              ) : (
                <Text type="secondary">{t('problem.noInspection')}</Text>
              )}
            </Descriptions.Item>
            <Descriptions.Item label={t('problem.inspector')}>
              <Text>{inspector ? inspector.display_name || inspector.username : `#${problem.inspector_id}`}</Text>
            </Descriptions.Item>
            <Descriptions.Item label={t('problem.capturedAt')}>
              <span className="tabular">{fmt(problem.captured_at)}</span>
            </Descriptions.Item>
            {problem.description ? (
              <Descriptions.Item label={t('problem.description')}>
                <Text>{problem.description}</Text>
              </Descriptions.Item>
            ) : null}
            {problem.note ? (
              <Descriptions.Item label={t('problem.note')}>
                <Text>{problem.note}</Text>
              </Descriptions.Item>
            ) : null}
          </Descriptions>

          <Divider orientation="left" style={{ margin: 0 }}>
            {t('problem.editSection')}
          </Divider>
          <ProblemEditForm problem={problem} canUpdate={canUpdate} />

          <Divider orientation="left" style={{ margin: 0 }}>
            <Space align="center" size={8}>
              <span>{t('problem.logSection')}</span>
              {canLog ? (
                <Button size="small" icon={<PlusOutlined />} onClick={() => setLogOpen(true)}>
                  {t('problem.addLog')}
                </Button>
              ) : null}
            </Space>
          </Divider>
          <ProblemProcessingLog problemId={problem.id} enabled={open} />

          <AddProcessingLogModal
            open={logOpen}
            problemId={problem.id}
            onClose={() => setLogOpen(false)}
          />
          <PanoramaModal
            open={panoOpen}
            mediaId={problem.cover_media_id ?? null}
            context={panoContext}
            onClose={() => setPanoOpen(false)}
          />
        </Space>
      )}
    </Drawer>
  )
}

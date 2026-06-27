import { Drawer, Tag, Timeline } from 'antd'
import { useTranslation } from 'react-i18next'
import type { AppConfig } from '@/shared/api/types'
import { EmptyState } from '@/shared/ui/EmptyState'
import { fmt } from '@/shared/time/dayjs'
import { stringifyJson } from '@/shared/util/jsonSchema'
import { useConfigHistory } from './api'

interface ConfigHistoryDrawerProps {
  open: boolean
  configKey: string | null
  onClose: () => void
}

export function ConfigHistoryDrawer({ open, configKey, onClose }: ConfigHistoryDrawerProps) {
  const { t } = useTranslation()
  const key = configKey ?? ''
  const { data, isLoading } = useConfigHistory(key, open && key.length > 0)
  // 后端按版本升序返回；展示时倒序（最新在上）。
  const versions: AppConfig[] = data ? [...data].reverse() : []

  return (
    <Drawer
      open={open}
      width={560}
      onClose={onClose}
      title={configKey ? `${t('dict.historyTitle')} · ${configKey}` : t('dict.historyTitle')}
      destroyOnHidden
      loading={isLoading}
    >
      {versions.length === 0 && !isLoading ? (
        <EmptyState description={t('dict.historyEmpty')} />
      ) : (
        <Timeline
          items={versions.map((v) => ({
            color: v.is_active ? 'green' : 'gray',
            children: (
              <div>
                <div style={{ marginBottom: 'var(--space-2)' }}>
                  <span className="tabular" style={{ fontWeight: 600 }}>
                    {t('dict.historyVersion')} v{v.version}
                  </span>
                  {v.is_active ? (
                    <Tag color="success" style={{ marginInlineStart: 8 }}>
                      {t('dict.historyCurrent')}
                    </Tag>
                  ) : null}
                </div>
                <div style={{ color: 'var(--color-ink-muted)', fontSize: 13 }}>
                  {t('dict.historyFrom')}：{fmt(v.effective_from)}
                </div>
                <div style={{ color: 'var(--color-ink-muted)', fontSize: 13 }}>
                  {t('dict.historyTo')}：{fmt(v.effective_to)}
                </div>
                <pre
                  className="tabular"
                  style={{
                    marginTop: 'var(--space-3)',
                    padding: 'var(--space-3)',
                    background: 'var(--color-canvas)',
                    borderRadius: 'var(--radius-sm)',
                    fontSize: 12,
                    overflowX: 'auto',
                  }}
                >
                  {stringifyJson(v.value)}
                </pre>
              </div>
            ),
          }))}
        />
      )}
    </Drawer>
  )
}

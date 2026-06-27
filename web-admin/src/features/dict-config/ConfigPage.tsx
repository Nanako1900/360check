import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { captureDefaultSchema, exportImageSchema } from '@/shared/util/jsonSchema'
import { ConfigCard } from './ConfigCard'
import { ConfigHistoryDrawer } from './ConfigHistoryDrawer'

/** 系统配置页：管理 export.image / capture.default（GET/PUT /config/{key} + 历史）。 */
export function ConfigPage() {
  const { t } = useTranslation()
  const [historyKey, setHistoryKey] = useState<string | null>(null)

  return (
    <>
      <div
        style={{
          display: 'grid',
          gap: 'var(--space-6)',
          gridTemplateColumns: 'repeat(auto-fit, minmax(360px, 1fr))',
        }}
      >
        <ConfigCard
          configKey="export.image"
          title={t('dict.configExportImage')}
          schema={exportImageSchema}
          onViewHistory={setHistoryKey}
        />
        <ConfigCard
          configKey="capture.default"
          title={t('dict.configCaptureDefault')}
          schema={captureDefaultSchema}
          onViewHistory={setHistoryKey}
        />
      </div>
      <ConfigHistoryDrawer
        open={historyKey !== null}
        configKey={historyKey}
        onClose={() => setHistoryKey(null)}
      />
    </>
  )
}

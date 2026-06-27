import { PictureOutlined } from '@ant-design/icons'
import { useMedia } from '@/features/panorama/api'

interface ProblemCoverProps {
  /** problems.cover_media_id（冗余封面，取 thumb 签名 URL，避免 N+1）。 */
  mediaId: number | null
}

const BOX = 48

/** 占位（无封面 / 加载中 / 取不到签名 URL）：中性图标，不报错。 */
function Placeholder() {
  return (
    <span
      aria-hidden
      style={{
        display: 'grid',
        placeItems: 'center',
        width: BOX,
        height: BOX,
        borderRadius: 6,
        background: 'var(--color-fill-quaternary, #f5f5f5)',
        color: 'var(--color-ink-muted, #bfbfbf)',
      }}
    >
      <PictureOutlined />
    </span>
  )
}

/**
 * 列表封面缩略图（§P6）：用 `cover_media_id` 取 thumb 签名 URL。
 * - 无封面 → 占位图标。
 * - 加载中 / 未确认 / 无签名 URL → 占位，绝不报错。
 * - 显式 width/height 防 CLS（§10）。alt 用问题封面，escape 由浏览器属性渲染保证。
 */
export function ProblemCover({ mediaId }: ProblemCoverProps) {
  const enabled = mediaId != null && mediaId > 0
  const { data: media } = useMedia(mediaId ?? 0, enabled)

  if (!enabled || !media?.signed_url) return <Placeholder />

  return (
    <img
      src={media.signed_url}
      alt=""
      width={BOX}
      height={BOX}
      loading="lazy"
      decoding="async"
      style={{ width: BOX, height: BOX, objectFit: 'cover', borderRadius: 6, display: 'block' }}
    />
  )
}

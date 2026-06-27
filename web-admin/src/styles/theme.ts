import type { ThemeConfig } from 'antd'
import { theme as antdTheme } from 'antd'

/**
 * 视觉方向：Stripe 式现代风格（refined SaaS / data-console）。
 * - 标志性 blurple 主色 #635BFF；精炼 slate 中性骨架；语义色专用于状态。
 * - 舒适留白、8px 圆角、层叠柔和阴影、Inter 字体、tabular-nums 数字。
 * - 纯视觉层（antd theme.token + cssVar）；不改任何行为。颜色对比满足 WCAG AA（§10.2）。
 */
export const BRAND_PRIMARY = '#635bff'
export const BRAND_PRIMARY_HOVER = '#7a73ff'
export const BRAND_PRIMARY_ACTIVE = '#5249e5'

// —— Stripe 中性与语义色 ——
const INK = '#1a1f36' // 主文本（深蓝墨）
const INK_SECONDARY = '#4f566b'
const INK_MUTED = '#697386'
const INK_TERTIARY = '#8792a2'
const CANVAS = '#f6f9fc' // 应用背景（极浅蓝灰）
const SURFACE = '#ffffff'
const HAIRLINE = '#e3e8ee'
const HAIRLINE_SOFT = '#edf0f5'
const SELECTED_TINT = 'rgba(99, 91, 255, 0.09)'

const SUCCESS = '#15a06b'
const WARNING = '#bb6f06'
const ERROR = '#df1b41'

// Stripe 层叠柔和阴影
const SHADOW_CARD = '0 1px 1px rgba(15,23,42,0.04), 0 2px 6px rgba(15,23,42,0.05)'
const SHADOW_FLOAT =
  '0 0 0 1px rgba(15,23,42,0.03), 0 6px 16px rgba(15,23,42,0.10), 0 2px 6px rgba(15,23,42,0.06)'
const FOCUS_RING = '0 0 0 3px rgba(99, 91, 255, 0.30)'

export const appTheme: ThemeConfig = {
  cssVar: true,
  algorithm: antdTheme.defaultAlgorithm,
  token: {
    colorPrimary: BRAND_PRIMARY,
    colorInfo: BRAND_PRIMARY,
    colorLink: BRAND_PRIMARY,
    colorLinkHover: BRAND_PRIMARY_HOVER,
    colorSuccess: SUCCESS,
    colorWarning: WARNING,
    colorError: ERROR,

    colorText: INK,
    colorTextSecondary: INK_SECONDARY,
    colorTextTertiary: INK_MUTED,
    colorTextQuaternary: INK_TERTIARY,

    colorBgLayout: CANVAS,
    colorBgContainer: SURFACE,
    colorBgElevated: SURFACE,
    colorBorder: HAIRLINE,
    colorBorderSecondary: HAIRLINE_SOFT,

    borderRadius: 8,
    borderRadiusLG: 12,
    borderRadiusSM: 6,
    controlHeight: 36,
    wireframe: false,

    fontSize: 14,
    fontFamily:
      '"Inter", -apple-system, BlinkMacSystemFont, "PingFang SC", "Microsoft YaHei", "Segoe UI", system-ui, sans-serif',
    fontFamilyCode: '"JetBrains Mono", "SFMono-Regular", Menlo, Consolas, monospace',

    boxShadow: SHADOW_CARD,
    boxShadowSecondary: SHADOW_FLOAT,
    lineWidth: 1,
  },
  components: {
    Layout: {
      headerBg: SURFACE,
      headerHeight: 60,
      headerPadding: '0 24px',
      siderBg: SURFACE,
      bodyBg: CANVAS,
    },
    Menu: {
      // 浅色侧边导航（Stripe 风：白底 + 靛蓝选中态）
      itemBg: 'transparent',
      subMenuItemBg: 'transparent',
      itemColor: INK_SECONDARY,
      itemHoverColor: INK,
      itemHoverBg: HAIRLINE_SOFT,
      itemSelectedBg: SELECTED_TINT,
      itemSelectedColor: BRAND_PRIMARY,
      itemBorderRadius: 8,
      itemMarginInline: 8,
      itemHeight: 40,
      activeBarWidth: 0,
      iconSize: 16,
    },
    Button: {
      controlHeight: 36,
      borderRadius: 8,
      fontWeight: 500,
      primaryShadow: '0 1px 2px rgba(26,31,54,0.16)',
      defaultShadow: '0 1px 2px rgba(26,31,54,0.06)',
      defaultBorderColor: '#d5dbe3',
      dangerShadow: '0 1px 2px rgba(223,27,65,0.16)',
    },
    Input: {
      controlHeight: 36,
      borderRadius: 8,
      activeShadow: FOCUS_RING,
      hoverBorderColor: BRAND_PRIMARY,
      activeBorderColor: BRAND_PRIMARY,
    },
    InputNumber: {
      controlHeight: 36,
      borderRadius: 8,
      activeShadow: FOCUS_RING,
    },
    Select: {
      controlHeight: 36,
      borderRadius: 8,
      optionSelectedBg: SELECTED_TINT,
      optionSelectedColor: BRAND_PRIMARY,
    },
    DatePicker: {
      controlHeight: 36,
      borderRadius: 8,
      activeShadow: FOCUS_RING,
    },
    Card: {
      borderRadiusLG: 12,
      boxShadowTertiary: SHADOW_CARD,
      headerFontSize: 15,
      colorBorderSecondary: HAIRLINE,
    },
    Table: {
      headerBg: '#f7fafc',
      headerColor: INK_MUTED,
      headerSplitColor: 'transparent',
      borderColor: HAIRLINE_SOFT,
      rowHoverBg: '#f6f9fc',
      cellPaddingBlock: 12,
      cellPaddingInline: 16,
      headerBorderRadius: 12,
    },
    Modal: {
      borderRadiusLG: 14,
      titleFontSize: 17,
    },
    Drawer: {
      colorBgElevated: SURFACE,
    },
    Tabs: {
      itemColor: INK_MUTED,
      itemSelectedColor: BRAND_PRIMARY,
      itemHoverColor: INK,
      inkBarColor: BRAND_PRIMARY,
      titleFontSize: 14,
    },
    Tag: {
      borderRadiusSM: 6,
    },
    Segmented: {
      itemSelectedBg: SURFACE,
      trackBg: HAIRLINE_SOFT,
      borderRadius: 8,
    },
    Progress: {
      defaultColor: BRAND_PRIMARY,
    },
    Statistic: {
      contentFontSize: 28,
    },
  },
}

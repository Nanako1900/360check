import type { ReactNode } from 'react'
import {
  BarChartOutlined,
  BookOutlined,
  DashboardOutlined,
  DownloadOutlined,
  PictureOutlined,
  ProjectOutlined,
  SafetyCertificateOutlined,
  ScheduleOutlined,
  WarningOutlined,
} from '@ant-design/icons'

/**
 * 菜单配置：path + i18n 名 + 图标 + 权限码（§P2 权限码目录）。
 * 权限码权威来源是后端 /auth/me 返回的 effective codes；前端只做 UI 过滤（真正鉴权在后端）。
 */
export interface AppMenuItem {
  path: string
  /** i18n key（menu.*） */
  name: string
  icon?: ReactNode
  /** 任一满足即显示；缺省表示无需权限。 */
  permission?: string | string[]
  children?: AppMenuItem[]
}

export const MENU: AppMenuItem[] = [
  { path: '/', name: 'menu.home', icon: <DashboardOutlined /> },
  {
    path: '/projects',
    name: 'menu.projects',
    icon: <ProjectOutlined />,
    permission: 'project:read',
  },
  {
    path: '/inspections',
    name: 'menu.inspections',
    icon: <ScheduleOutlined />,
    permission: 'inspection:read',
  },
  {
    path: '/problems',
    name: 'menu.problems',
    icon: <WarningOutlined />,
    permission: 'problem:read',
  },
  { path: '/panorama', name: 'menu.panorama', icon: <PictureOutlined />, permission: 'media:read' },
  { path: '/stats', name: 'menu.stats', icon: <BarChartOutlined />, permission: 'stats:read' },
  { path: '/exports', name: 'menu.exports', icon: <DownloadOutlined />, permission: 'export:read' },
  {
    path: '/dict',
    name: 'menu.dict',
    icon: <BookOutlined />,
    permission: ['dict:read', 'config:read'],
  },
  {
    path: '/rbac',
    name: 'menu.rbac',
    icon: <SafetyCertificateOutlined />,
    permission: ['user:read', 'role:read', 'permission:read'],
  },
]

/** 判断单个菜单项是否对当前权限可见（任一权限满足）。 */
export function isMenuItemVisible(item: AppMenuItem, can: (code: string) => boolean): boolean {
  if (!item.permission) return true
  const codes = Array.isArray(item.permission) ? item.permission : [item.permission]
  return codes.length === 0 || codes.some(can)
}

/**
 * 纯函数：按权限谓词递归过滤菜单（可单测）。父项无权限则整支隐藏；
 * 父项有权限但所有子项被过滤时保留父项自身（仍可点击）。
 */
export function filterMenu(items: AppMenuItem[], can: (code: string) => boolean): AppMenuItem[] {
  const out: AppMenuItem[] = []
  for (const item of items) {
    if (!isMenuItemVisible(item, can)) continue
    const children = item.children ? filterMenu(item.children, can) : undefined
    out.push(children ? { ...item, children } : { ...item })
  }
  return out
}

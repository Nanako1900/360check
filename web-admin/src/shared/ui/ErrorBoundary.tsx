import { Component } from 'react'
import type { ErrorInfo, ReactNode } from 'react'
import { Button, Result } from 'antd'
import i18n from '@/shared/i18n'

interface ErrorBoundaryProps {
  children: ReactNode
  fallback?: ReactNode
}

interface ErrorBoundaryState {
  hasError: boolean
  message: string
}

/** 全局渲染错误兜底（§P0）。捕获子树渲染异常 → 友好卡片，控制台留详细上下文，绝不静默吞错。 */
export class ErrorBoundary extends Component<ErrorBoundaryProps, ErrorBoundaryState> {
  constructor(props: ErrorBoundaryProps) {
    super(props)
    this.state = { hasError: false, message: '' }
  }

  static getDerivedStateFromError(error: unknown): ErrorBoundaryState {
    return { hasError: true, message: error instanceof Error ? error.message : String(error) }
  }

  override componentDidCatch(error: Error, info: ErrorInfo): void {
    // 详细上下文仅落控制台 / 上报（不向用户泄露）
    console.error('[ErrorBoundary]', error, info.componentStack)
  }

  private handleReload = (): void => {
    window.location.reload()
  }

  override render(): ReactNode {
    if (this.state.hasError) {
      if (this.props.fallback) return this.props.fallback
      return (
        <Result
          status="error"
          title={i18n.t('error.boundaryTitle')}
          subTitle={i18n.t('error.boundarySubtitle')}
          extra={
            <Button type="primary" onClick={this.handleReload}>
              {i18n.t('common.refresh')}
            </Button>
          }
        />
      )
    }
    return this.props.children
  }
}

import { describe, expect, it, vi } from 'vitest'
import { triggerDownload } from './download'

describe('triggerDownload', () => {
  it('creates a hidden anchor with the URL and a download attr, clicks, then removes it', () => {
    const click = vi.fn()
    const appended: HTMLAnchorElement[] = []
    const anchor = {
      href: '',
      rel: '',
      style: { display: '' },
      setAttribute: vi.fn(function (this: { _download?: string }, name: string, value: string) {
        if (name === 'download') this._download = value
      }),
      click,
    } as unknown as HTMLAnchorElement

    const doc = {
      createElement: vi.fn(() => anchor),
      body: {
        appendChild: vi.fn((el: HTMLAnchorElement) => appended.push(el)),
        removeChild: vi.fn(),
      },
    } as unknown as Document

    triggerDownload('https://cdn.example.com/result.xlsx?sig=abc', doc)

    expect(doc.createElement).toHaveBeenCalledWith('a')
    expect(anchor.href).toBe('https://cdn.example.com/result.xlsx?sig=abc')
    expect(anchor.setAttribute).toHaveBeenCalledWith('download', '')
    expect(click).toHaveBeenCalledOnce()
    expect(doc.body.appendChild).toHaveBeenCalledWith(anchor)
    expect(doc.body.removeChild).toHaveBeenCalledWith(anchor)
  })

  it('is a no-op for an empty URL', () => {
    const createElement = vi.fn()
    const doc = { createElement } as unknown as Document
    triggerDownload('', doc)
    expect(createElement).not.toHaveBeenCalled()
  })
})

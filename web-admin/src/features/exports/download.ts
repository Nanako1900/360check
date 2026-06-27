/**
 * 触发浏览器下载签名 CDN URL（§P8）。
 *
 * `result_url` 是后端给出的时效签名 CDN URL（指向 .xlsx）。用隐藏 <a download> 锚点点击触发，
 * 不污染当前页历史；`document` 可注入便于单测断言。无 `dangerouslySetInnerHTML`（§7）。
 */
export function triggerDownload(url: string, doc: Document = globalThis.document): void {
  if (!url) return
  // SECURITY (FE-M2): only http(s) — never javascript:/data:/blob:, which an
  // anchor click would execute or navigate to. result_url is a trusted signed CDN
  // URL today, but guard at the sink so this helper is safe if ever reused.
  if (!/^https?:\/\//i.test(url)) return
  const anchor = doc.createElement('a')
  anchor.href = url
  anchor.rel = 'noopener'
  // download 提示浏览器另存；跨域签名 URL 文件名以 Content-Disposition 为准。
  anchor.setAttribute('download', '')
  anchor.style.display = 'none'
  doc.body.appendChild(anchor)
  anchor.click()
  doc.body.removeChild(anchor)
}

/// <reference types="vite/client" />

interface ImportMetaEnv {
  readonly VITE_API_BASE_URL: string
  readonly VITE_MAP_KEY: string
  readonly VITE_CDN_BASE: string
  readonly VITE_ENABLE_MSW: string
}

interface ImportMeta {
  readonly env: ImportMetaEnv
}

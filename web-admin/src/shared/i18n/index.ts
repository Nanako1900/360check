import i18n from 'i18next'
import { initReactI18next } from 'react-i18next'
import { zhCN } from './zh-CN'

void i18n.use(initReactI18next).init({
  resources: { 'zh-CN': zhCN },
  lng: 'zh-CN',
  fallbackLng: 'zh-CN',
  interpolation: { escapeValue: false }, // React 自动转义，避免 XSS（§7）
  returnNull: false,
})

export default i18n

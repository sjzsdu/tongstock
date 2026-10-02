import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import { App as AntApp, ConfigProvider, theme } from 'antd'
import './index.css'
import App from './App.tsx'
import { PRICE_PALETTE } from './lib/palette'

// 涨跌色统一来源：lib/palette.ts。CSS 侧（.price-up/.price-down 等）
// 通过这里的 CSS 变量引用同一套值，避免 TS 与 CSS 各写一份色值。
const rootStyle = document.documentElement.style
rootStyle.setProperty('--price-up', PRICE_PALETTE.up)
rootStyle.setProperty('--price-down', PRICE_PALETTE.down)
rootStyle.setProperty('--price-flat', PRICE_PALETTE.flat)

createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <ConfigProvider
      theme={{
        algorithm: theme.darkAlgorithm,
        token: {
          colorPrimary: '#1677ff',
          colorSuccess: '#22c55e',
          colorError: '#ef4444',
          colorWarning: '#f59e0b',
          borderRadius: 10,
          colorBgBase: '#0b1220',
          colorBgContainer: '#111827',
          colorBgElevated: '#111827',
          colorBorder: '#1f2937',
        },
        components: {
          Layout: {
            siderBg: '#0f172a',
            bodyBg: '#0b1220',
            headerBg: '#0b1220',
            triggerBg: '#0f172a',
          },
          Menu: {
            darkItemBg: '#0f172a',
            darkSubMenuItemBg: '#0f172a',
            darkItemSelectedBg: '#1677ff',
            darkItemHoverBg: '#1e293b',
          },
          Card: {
            bodyPadding: 20,
          },
        },
      }}
    >
      <AntApp>
        <App />
      </AntApp>
    </ConfigProvider>
  </StrictMode>,
)

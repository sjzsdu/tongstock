import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import { App as AntApp, ConfigProvider } from 'antd'
import './index.css'
import App from './App.tsx'
import { PRICE_PALETTE } from './lib/palette'
import { applyRestoredAppPath } from './lib/appPath'
// 主题配置单点：main.tsx 与组件测试共用同一份深色主题（见 theme/appTheme.ts）
import { APP_THEME } from './theme/appTheme'

// 静态托管（GitHub Pages）下 `/app/some/deep/link` 会被根 404 兜底脚本跳到
// `/app/?__app_path=some/deep/link`，这里在挂载前把地址还原，router 才能落到目标页。
// 非兜底进入时它什么都不做（返回 null）。
applyRestoredAppPath()

// 涨跌色统一来源：lib/palette.ts。CSS 侧（.price-up/.price-down 等）
// 通过这里的 CSS 变量引用同一套值，避免 TS 与 CSS 各写一份色值。
const rootStyle = document.documentElement.style
rootStyle.setProperty('--price-up', PRICE_PALETTE.up)
rootStyle.setProperty('--price-down', PRICE_PALETTE.down)
rootStyle.setProperty('--price-flat', PRICE_PALETTE.flat)

createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <ConfigProvider theme={APP_THEME}>
      <AntApp>
        <App />
      </AntApp>
    </ConfigProvider>
  </StrictMode>,
)

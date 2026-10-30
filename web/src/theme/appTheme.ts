import { theme, type ThemeConfig } from 'antd';

/**
 * 全站唯一的 antd 主题配置。
 *
 * 单独抽成模块的原因是：组件测试要拿「真实站点用的那套深色主题」去渲染，
 * 才能断言颜色在深色底上真的读得清（写死一份浅色预设色在深色底上往往只有 2:1）。
 * main.tsx 与测试都引用这里，避免两处各写一份 token 而慢慢漂移。
 */
export const APP_THEME: ThemeConfig = {
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
};

/** 卡片内容区的底色，组件里做对比度自检时的参照面 */
export const CARD_SURFACE = APP_THEME.token?.colorBgContainer as string;

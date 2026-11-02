import { Flex, Spin, Typography } from 'antd';

type PageLoadingProps = {
  /** 首屏占位高度。默认值让 spinner 落在内容区视觉中心而不是贴着左上角。 */
  minHeight?: number | string;
  size?: 'small' | 'default' | 'large';
  tip?: string;
};

/**
 * 页面级加载占位：spinner 与提示文字整体居中。
 *
 * 直接写 `<Spin />` 时它是 inline-block，会贴住内容区左上角，在深色背景上
 * 看起来像"页面坏了"。所有页面级 loading 都走这里，保证位置与尺寸一致。
 */
export default function PageLoading({ minHeight = 'max(220px, calc(100vh - 380px))', size = 'large', tip }: PageLoadingProps) {
  return (
    <Flex
      vertical
      align="center"
      justify="center"
      gap={12}
      role="status"
      aria-live="polite"
      style={{ minHeight, width: '100%' }}
    >
      <Spin size={size} />
      {tip && (
        <Typography.Text type="secondary" style={{ fontSize: 13 }}>
          {tip}
        </Typography.Text>
      )}
    </Flex>
  );
}

import { Radio, Space, Typography } from 'antd';

interface Props {
  ktype: string;
  onKtypeChange: (ktype: string) => void;
  mainOverlay: string;
  onMainOverlayChange: (v: string) => void;
  subPanel: string;
  onSubPanelChange: (v: string) => void;
}

const KTYPES = [
  { value: '1m', label: '1分' },
  { value: '5m', label: '5分' },
  { value: '15m', label: '15分' },
  { value: '30m', label: '30分' },
  { value: '60m', label: '60分' },
  { value: 'day', label: '日K' },
  { value: 'week', label: '周K' },
  { value: 'month', label: '月K' },
];

const MAIN_OVERLAYS = [
  { value: 'MA', label: 'MA' },
  { value: 'BOLL', label: 'BOLL' },
];

const SUB_PANELS = [
  { value: 'MACD', label: 'MACD' },
  { value: 'KDJ', label: 'KDJ' },
  { value: 'RSI', label: 'RSI' },
];

const labelStyle = { whiteSpace: 'nowrap' } as const;

export default function ChartToolbar({
  ktype,
  onKtypeChange,
  mainOverlay,
  onMainOverlayChange,
  subPanel,
  onSubPanelChange,
}: Props) {
  return (
    <div style={{ overflowX: 'auto', paddingBottom: 2 }}>
      <Space size={18} align="center" wrap={false} style={{ minWidth: 'max-content' }}>
        <Space size={8} align="center" wrap={false}>
          <Typography.Text type="secondary" style={labelStyle}>
            K线
          </Typography.Text>
          <Radio.Group
            size="small"
            optionType="button"
            buttonStyle="solid"
            options={KTYPES}
            value={ktype}
            onChange={(event) => onKtypeChange(event.target.value)}
          />
        </Space>
        <Space size={8} align="center" wrap={false}>
          <Typography.Text type="secondary" style={labelStyle}>
            主图
          </Typography.Text>
          <Radio.Group
            size="small"
            optionType="button"
            buttonStyle="solid"
            options={MAIN_OVERLAYS}
            value={mainOverlay || undefined}
            onChange={(event) => onMainOverlayChange(event.target.value === mainOverlay ? '' : event.target.value)}
          />
        </Space>
        <Space size={8} align="center" wrap={false}>
          <Typography.Text type="secondary" style={labelStyle}>
            副图
          </Typography.Text>
          <Radio.Group
            size="small"
            optionType="button"
            buttonStyle="solid"
            options={SUB_PANELS}
            value={subPanel || undefined}
            onChange={(event) => onSubPanelChange(event.target.value === subPanel ? '' : event.target.value)}
          />
        </Space>
      </Space>
    </div>
  );
}

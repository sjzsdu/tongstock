import { Modal, Button, Space, Typography, Tag, Alert } from 'antd';
import type { DimensionMeta } from '../../types/api';

interface SignalHelpModalProps {
  visible: boolean;
  onCancel: () => void;
  dimensions: DimensionMeta[];
}

export function SignalHelpModal({ visible, onCancel, dimensions }: SignalHelpModalProps) {
  return (
    <Modal title="三态筛选说明" open={visible} onCancel={onCancel} footer={<Button onClick={onCancel}>关闭</Button>} width={680}>
      <Space orientation="vertical" size={16} style={{ width: '100%' }}>
        {dimensions.map((dim) => <div key={dim.dim}>
          <Typography.Text strong>{dim.label}</Typography.Text>
          <Typography.Paragraph>{dim.description}</Typography.Paragraph>
          {dim.states.map((s) => <div key={s.state}>
            <Tag color={s.color}>{s.label}（{s.direction_label}）</Tag><Typography.Text>{s.description}</Typography.Text>
          </div>)}
        </div>)}
        <Alert type="info" showIcon title="全部条件同时满足（AND）" description="状态来自所选交易日的四维快照。中性须明确选择，不限定表示该维不加条件。任一维数据不足的股日均被排除并计数。迁移指窗口内相邻快照由起态转为终态，窗口按自然日计算，包含所选日；变化后的状态无需一直维持到所选日，可另加当日状态条件。" />
      </Space>
    </Modal>
  );
}

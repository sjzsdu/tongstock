import { Modal, Button, Space, Typography, Divider, Tag, Alert } from 'antd';
import { SIGNAL_OPTIONS } from '../../types/screen';

const { Text } = Typography;

interface SignalHelpModalProps {
  visible: boolean;
  onCancel: () => void;
}

export function SignalHelpModal({ visible, onCancel }: SignalHelpModalProps) {
  return (
    <Modal
      title="信号含义说明"
      open={visible}
      onCancel={onCancel}
      footer={<Button onClick={onCancel}>关闭</Button>}
      width={680}
    >
      <Space direction="vertical" size={16} style={{ width: '100%' }}>
        <div>
          <Text strong style={{ fontSize: 14 }}>📈 买入信号</Text>
          <Divider style={{ margin: '8px 0' }} />
          <Space direction="vertical" size={12} style={{ width: '100%' }}>
            {SIGNAL_OPTIONS.filter((opt) => opt.buy).map((opt) => (
              <div key={opt.value} style={{ display: 'flex', gap: 12 }}>
                <Tag color="red" style={{ flexShrink: 0 }}>{opt.label}</Tag>
                <Text>{opt.desc}</Text>
              </div>
            ))}
          </Space>
        </div>

        <div>
          <Text strong style={{ fontSize: 14 }}>📉 卖出信号</Text>
          <Divider style={{ margin: '8px 0' }} />
          <Space direction="vertical" size={12} style={{ width: '100%' }}>
            {SIGNAL_OPTIONS.filter((opt) => !opt.buy).map((opt) => (
              <div key={opt.value} style={{ display: 'flex', gap: 12 }}>
                <Tag color="green" style={{ flexShrink: 0 }}>{opt.label}</Tag>
                <Text>{opt.desc}</Text>
              </div>
            ))}
          </Space>
        </div>

        <Alert
          type="info"
          showIcon
          message="筛选逻辑说明"
          description="选择多个信号时，只要股票满足其中任意一个信号条件就会被列入结果。表格中显示的是该股票最近触发的信号。"
          style={{ marginTop: 8 }}
        />
      </Space>
    </Modal>
  );
}
import { Modal, Button, Collapse, Space, Typography, Tag, Alert } from 'antd';
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
        <Collapse
          size="small"
          items={[
            {
              key: 'daily-state',
              label: '每日状态是什么',
              children: (
                <Space orientation="vertical" size={6}>
                  <Typography.Text>
                    每个交易日收盘后，系统按当日行情给全市场每只股票算出四个指标的状态并存下来，筛选就是在这一天的状态表里找同时满足条件的股票。
                  </Typography.Text>
                  <Typography.Text type="secondary">
                    状态在收盘后算好，只用当日及之前的日线（未复权），不会看到未来数据；同一状态日、同一组条件，重跑结果完全一致。
                  </Typography.Text>
                </Space>
              ),
            },
            {
              key: 'skipped',
              label: '哪些股票不会出现在结果里',
              children: (
                <Space orientation="vertical" size={6}>
                  <Typography.Text>
                    1. <Typography.Text strong>那天没有交易</Typography.Text>：停牌、还没上市、已经退市。它没有当日行情，也就没有当日状态，这不是数据出错。
                  </Typography.Text>
                  <Typography.Text>
                    2. <Typography.Text strong>历史太短，指标算不出来</Typography.Text>：新股上市头几十个交易日，MACD 这类指标需要足够长的历史，界面标记为「数据不足」。
                  </Typography.Text>
                  <Typography.Text>
                    3. <Typography.Text strong>那天状态还没算出来</Typography.Text>：本地行情已经有了，但状态尚未生成。这种情况可以点「更新数据」，补齐后自动重新筛选。
                  </Typography.Text>
                </Space>
              ),
            },
          ]}
        />
        {dimensions.map((dim) => <div key={dim.dim}>
          <Typography.Text strong>{dim.label}</Typography.Text>
          <Typography.Paragraph>{dim.description}</Typography.Paragraph>
          {dim.states.map((s) => <div key={s.state}>
            <Tag color={s.color}>{s.label}（{s.direction_label}）</Tag><Typography.Text>{s.description}</Typography.Text>
          </div>)}
        </div>)}
        <Alert type="info" showIcon title="全部条件同时满足（AND）" description="状态来自所选状态日的每日状态。中性须明确选择，不限定表示该维不加条件。任一维数据不足的股票不参与筛选，并在覆盖情况里单独计数。迁移指窗口内相邻状态日由起态转为终态，窗口按自然日计算，包含所选日；变化后的状态无需一直维持到所选日，可另加当日状态条件。" />
      </Space>
    </Modal>
  );
}

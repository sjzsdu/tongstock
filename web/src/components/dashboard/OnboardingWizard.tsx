import { useState } from 'react';
import { Alert, Button, Card, Collapse, Space, Steps, Tag, Typography, message } from 'antd';
import { CheckCircleOutlined, CloseCircleOutlined, MinusCircleOutlined, ThunderboltOutlined } from '@ant-design/icons';
import { api, type OnboardingResult, type OnboardingStep } from '../../api/client';
import MethodMechanismDiagram from './MethodMechanismDiagram';

const { Text, Paragraph } = Typography;

/** 首次引导要讲清的 4 件事。 */
const GUIDE_STEPS = [
  { title: '同步真实行情', desc: '把最新交易日之前的真实日线补进本地库。' },
  { title: '系统自动冻结快照', desc: '把行情固化成不可变快照，选股只看这份冻结数据。' },
  { title: '看一份真实候选', desc: '让已验证的方法在今天的数据上判定，得到今日候选。' },
  { title: '理解方法机制', desc: '研究 → 验证 → 选股 → 卖出，四步走完一个闭环。' },
];

const STEP_ICON: Record<OnboardingStep['status'], React.ReactNode> = {
  done: <CheckCircleOutlined style={{ color: '#52c41a' }} />,
  skipped: <MinusCircleOutlined style={{ color: '#8c8c8c' }} />,
  blocked: <CloseCircleOutlined style={{ color: '#faad14' }} />,
  failed: <CloseCircleOutlined style={{ color: '#ff4d4f' }} />,
};

const STEP_TAG: Record<OnboardingStep['status'], { color: string; label: string }> = {
  done: { color: 'green', label: '已完成' },
  skipped: { color: 'default', label: '已就绪' },
  blocked: { color: 'orange', label: '被阻断' },
  failed: { color: 'red', label: '失败' },
};

/**
 * P3・首次引导：把「要用户自己摸索的流程」变成「系统带着走」。
 *
 * 「一键走通」调用后端编排接口，真正执行：行情检查/同步 → 冻结市场快照 →
 * 物化特征快照 → 运行今日选股。每一步都回传真实状态；被真实条件阻断时
 * 如实说明原因，不伪造「已完成」。
 */
export default function OnboardingWizard({ onCompleted }: { onCompleted?: () => void }) {
  const [running, setRunning] = useState(false);
  const [result, setResult] = useState<OnboardingResult | null>(null);

  const run = async (sync: boolean) => {
    setRunning(true);
    try {
      const res = await api.onboardingRun({ sync });
      setResult(res);
      if (res.status === 'completed') {
        void message.success(`走通完成：扫描 ${res.scanned_stocks} 只，得到 ${res.candidate_count} 个候选`);
        onCompleted?.();
      } else {
        void message.warning(res.blocked_reason || '走通被真实条件阻断');
      }
    } catch (e) {
      void message.error(e instanceof Error ? e.message : '一键走通失败');
    } finally {
      setRunning(false);
    }
  };

  return (
    <Card
      bordered={false}
      style={{ borderLeft: '3px solid #1677ff' }}
      title={
        <Space>
          <ThunderboltOutlined style={{ color: '#1677ff' }} />
          <Text strong>第一次使用？跟着走一遍</Text>
        </Space>
      }
    >
      <Space direction="vertical" size={12} style={{ display: 'flex' }}>
        <Paragraph type="secondary" style={{ margin: 0 }}>
          不用自己摸索。点「一键走通」，系统会依次执行下面四步，直接落到今日决策页。
        </Paragraph>

        <Steps
          direction="vertical"
          size="small"
          current={result ? GUIDE_STEPS.length : -1}
          items={GUIDE_STEPS.map((s) => ({ title: s.title, description: s.desc }))}
        />

        <Space size={12} wrap>
          <Button
            type="primary"
            icon={<ThunderboltOutlined />}
            loading={running}
            onClick={() => void run(false)}
          >
            一键走通
          </Button>
          <Button loading={running} onClick={() => void run(true)}>
            先同步行情再走通
          </Button>
        </Space>

        {result && (
          <Alert
            type={result.status === 'completed' ? 'success' : 'warning'}
            showIcon
            message={
              result.status === 'completed'
                ? `走通完成 · ${result.trade_date ?? ''}`
                : `走通未完成：${result.blocked_reason || result.status}`
            }
            description={
              <Space direction="vertical" size={4} style={{ display: 'flex' }}>
                {result.steps.map((step) => {
                  const tag = STEP_TAG[step.status];
                  return (
                    <Space key={step.key} size={8}>
                      {STEP_ICON[step.status]}
                      <Text>{step.label}</Text>
                      <Tag color={tag.color}>{tag.label}</Tag>
                      {step.detail && (
                        <Text type="secondary" style={{ fontSize: 12 }}>
                          {step.detail}
                        </Text>
                      )}
                    </Space>
                  );
                })}
              </Space>
            }
            action={
              result.status === 'completed' ? (
                <Button size="small" type="primary" onClick={() => onCompleted?.()}>
                  查看今日决策
                </Button>
              ) : undefined
            }
          />
        )}

        <Collapse
          ghost
          size="small"
          items={[
            {
              key: 'mechanism',
              label: '第 4 步：这套工具怎么思考？（图解）',
              children: <MethodMechanismDiagram />,
            },
          ]}
        />
      </Space>
    </Card>
  );
}
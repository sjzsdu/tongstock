import { useEffect, useMemo, useState } from 'react';
import {
  AlertOutlined,
  BulbOutlined,
  InfoCircleOutlined,
  RiseOutlined,
  WarningOutlined,
} from '@ant-design/icons';
import {
  Alert,
  Card,
  Collapse,
  List,
  Pagination,
  Space,
  Tag,
  Typography,
} from 'antd';
import type { SignalWithInterpretation } from '../types/api';

interface SignalInterpretationCardProps {
  interpretations: SignalWithInterpretation[];
  overallSummary: string;
  trend: string;
}

function getRiskColor(level: string) {
  switch (level) {
    case 'low':
      return 'green';
    case 'medium':
      return 'orange';
    case 'high':
      return 'red';
    default:
      return 'default';
  }
}

function getRiskIcon(level: string) {
  switch (level) {
    case 'low':
      return <RiseOutlined style={{ color: '#22c55e' }} />;
    case 'medium':
      return <InfoCircleOutlined style={{ color: '#f97316' }} />;
    case 'high':
      return <WarningOutlined style={{ color: '#ef4444' }} />;
    default:
      return <AlertOutlined />;
  }
}

function getSignalTypeColor(type: string) {
  if (type.includes('金叉') || type.includes('超卖') || type.includes('突破') || type.includes('多头')) {
    return 'red';
  }
  if (type.includes('死叉') || type.includes('超买') || type.includes('跌破') || type.includes('空头')) {
    return 'green';
  }
  return 'default';
}

function compareSignalDateDesc(a: SignalWithInterpretation, b: SignalWithInterpretation): number {
  return String(b.signal?.date ?? '').localeCompare(String(a.signal?.date ?? ''));
}

const PAGE_SIZE = 20;

export default function SignalInterpretationCard({
  interpretations,
  overallSummary,
	trend,
}: SignalInterpretationCardProps) {
  const [page, setPage] = useState(1);
  const sortedInterpretations = useMemo(
    () => [...(Array.isArray(interpretations) ? interpretations : [])].sort(compareSignalDateDesc),
    [interpretations],
  );
  const pagedInterpretations = sortedInterpretations.slice((page - 1) * PAGE_SIZE, page * PAGE_SIZE);

  useEffect(() => {
    setPage(1);
  }, [interpretations]);

	if (sortedInterpretations.length === 0) {
    return (
      <Card>
        <Alert
          type="info"
          message="当前无明显技术信号"
          description={`当前趋势：${trend}`}
          showIcon
        />
      </Card>
    );
  }

  return (
    <Space direction="vertical" size={16} style={{ display: 'flex' }}>
      {/* 综合摘要 */}
      <Card size="small" style={{ background: 'rgba(22,119,255,0.08)' }}>
        <Space direction="vertical" size={8} style={{ display: 'flex' }}>
          <Space>
            <BulbOutlined style={{ color: '#1677ff' }} />
            <Typography.Text strong>信号摘要</Typography.Text>
          </Space>
          <Typography.Text>{overallSummary}</Typography.Text>
          <Space>
            <Typography.Text type="secondary">当前趋势：</Typography.Text>
            <Tag color={trend === '上涨趋势' ? 'red' : trend === '下跌趋势' ? 'green' : 'default'}>
              {trend}
            </Tag>
          </Space>
        </Space>
      </Card>

      {/* 信号解读列表 */}
			<Card title={`信号解读 (${sortedInterpretations.length})`}>
				<Collapse
					accordion
					items={pagedInterpretations.map((item, index) => ({
						key: `${item.signal?.date ?? ''}-${item.signal?.indicator ?? ''}-${item.signal?.type ?? ''}-${index}`,
						label: (
              <Space>
                <Tag color={getSignalTypeColor(item.signal.type)}>
                  {item.signal.type}
                </Tag>
                <Typography.Text>{item.signal.indicator}</Typography.Text>
                <Typography.Text type="secondary" style={{ fontSize: 12 }}>
                  {item.signal.date}
                </Typography.Text>
                {getRiskIcon(item.interpretation.risk_level)}
              </Space>
            ),
            children: (
              <Space direction="vertical" size={12} style={{ display: 'flex' }}>
                {/* 概要 */}
                <div>
                  <Typography.Text strong style={{ marginRight: 8 }}>解读：</Typography.Text>
                  <Typography.Text>{item.interpretation.summary}</Typography.Text>
                </div>

                {/* 详细解释 */}
                <div>
                  <Typography.Text type="secondary">{item.interpretation.explanation}</Typography.Text>
                </div>

                {/* 操作建议 */}
                {item.interpretation.suggestions.length > 0 && (
                  <div>
                    <Typography.Text strong style={{ marginRight: 8 }}>建议：</Typography.Text>
                    <List
                      size="small"
                      dataSource={item.interpretation.suggestions}
                      renderItem={(suggestion) => (
                        <List.Item style={{ padding: '4px 0', border: 'none' }}>
                          <Typography.Text type="secondary">• {suggestion}</Typography.Text>
                        </List.Item>
                      )}
                    />
                  </div>
                )}

                {/* 风险等级 */}
                <Space>
                  <Typography.Text type="secondary">风险等级：</Typography.Text>
                  <Tag color={getRiskColor(item.interpretation.risk_level)}>
                    {item.interpretation.risk_level.toUpperCase()}
                  </Tag>
                </Space>

                {/* 信号强度 */}
                <Space>
                  <Typography.Text type="secondary">信号强度：</Typography.Text>
                  <Typography.Text>
                    {item.signal.strength.toFixed(2)}
                  </Typography.Text>
                </Space>
              </Space>
            ),
					}))}
				/>
				{sortedInterpretations.length > PAGE_SIZE && (
					<div style={{ marginTop: 16, textAlign: 'right' }}>
						<Pagination
							current={page}
							pageSize={PAGE_SIZE}
							total={sortedInterpretations.length}
							showSizeChanger={false}
							showTotal={(total) => `共 ${total} 条`}
							onChange={setPage}
						/>
					</div>
				)}
			</Card>
    </Space>
  );
}

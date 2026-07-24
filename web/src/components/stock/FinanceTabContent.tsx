import { useRef } from 'react';
import { Button, Card, Checkbox, Col, Descriptions, Empty, Flex, Radio, Row, Segmented, Spin, Space, Statistic, Table, Tabs, Typography, message } from 'antd';
import { BarChartOutlined, CameraOutlined, FileExcelOutlined } from '@ant-design/icons';
import type { ColumnsType } from 'antd/es/table';
import type { FinanceTrendRecord, FinanceMetricsResponse } from '../../types/api';
import type { FinanceTrendMetric, FinanceCompareMode, FinanceViewMode } from '../../hooks/useStockFinance';
import FinanceTrendChart, { type FinanceTrendChartHandle } from '../charts/FinanceTrendChart';
import { exportCsv, downloadDataUrl } from '../../lib/stock-detail';
import { formatDate } from '../../lib/datetime';

interface FinanceTabContentProps {
  financeTrends: any;
  financeMetrics: FinanceMetricsResponse | null;
  financeTrendMode: 'quarter' | 'year';
  setFinanceTrendMode: (mode: 'quarter' | 'year') => void;
  financeCompareMode: FinanceCompareMode;
  setFinanceCompareMode: (mode: FinanceCompareMode) => void;
  financeViewMode: FinanceViewMode;
  setFinanceViewMode: (mode: FinanceViewMode) => void;
  selectedFinanceMetrics: string[];
  setSelectedFinanceMetrics: (metrics: string[]) => void;
  financeTrendLoading: boolean;
  availableFinanceMetrics: FinanceTrendMetric[];
  financeChartGroups: any[];
  financeDisplayRecords: FinanceTrendRecord[];
  activeFinanceMetrics: FinanceTrendMetric[];
  latestFinanceRecord: FinanceTrendRecord | undefined;
  formatFinanceMetricValue: (value: number | undefined, metric: FinanceTrendMetric) => string;
  financeItems: any[][];
  code: string;
}

export function FinanceTabContent({
  financeTrends,
  financeMetrics,
  financeTrendMode,
  setFinanceTrendMode,
  financeCompareMode,
  setFinanceCompareMode,
  financeViewMode,
  setFinanceViewMode,
  selectedFinanceMetrics,
  setSelectedFinanceMetrics,
  financeTrendLoading,
  availableFinanceMetrics,
  financeChartGroups,
  financeDisplayRecords,
  activeFinanceMetrics,
  latestFinanceRecord,
  formatFinanceMetricValue,
  financeItems,
  code,
}: FinanceTabContentProps) {
  const amountChartRef = useRef<FinanceTrendChartHandle | null>(null);
  const marginChartRef = useRef<FinanceTrendChartHandle | null>(null);

  const handleExportFinanceCsv = () => {
    if (financeDisplayRecords.length === 0 || activeFinanceMetrics.length === 0) {
      message.warning('暂无可导出的财务趋势数据');
      return;
    }
    exportCsv(
      `${code}-finance-${financeTrendMode}-${financeCompareMode}.csv`,
      ['期间', ...activeFinanceMetrics.map((metric) => metric.label)],
      financeDisplayRecords.map((record) => [
        record.label,
        ...activeFinanceMetrics.map((metric) => {
          const axis = financeCompareMode === 'raw' ? metric.axis : 'percent';
          return formatFinanceMetricValue(record[metric.key as keyof FinanceTrendRecord] as number | undefined, { ...metric, axis });
        }),
      ]),
    );
    message.success('已导出财务趋势数据 CSV');
  };

  const handleExportFinanceChart = (groupKey: 'amount' | 'margin') => {
    const chartRef = groupKey === 'amount' ? amountChartRef.current : marginChartRef.current;
    const image = chartRef?.exportImage();
    if (!image) {
      message.warning('当前图表尚未准备好，稍后再试');
      return;
    }
    downloadDataUrl(`${code}-finance-${groupKey}-${financeTrendMode}-${financeCompareMode}.png`, image);
    message.success('已导出财务趋势图');
  };

  const financeTableColumns: ColumnsType<FinanceTrendRecord> = [
    {
      title: financeTrendMode === 'year' ? '年度' : '期间',
      dataIndex: 'label',
      fixed: 'left',
      width: 120,
    },
    ...activeFinanceMetrics.map((metric) => ({
      title: metric.label,
      key: metric.key,
      align: 'right' as const,
      render: (_: unknown, row: FinanceTrendRecord) => formatFinanceMetricValue(row[metric.key as keyof FinanceTrendRecord] as number | undefined, {
        ...metric,
        axis: financeCompareMode === 'raw' ? metric.axis : 'percent',
      }),
    })),
  ];

  return (
    <Space direction="vertical" size={16} style={{ display: 'flex' }}>
      <Card title="财务概览">
        <Descriptions bordered column={{ xs: 1, md: 2, xl: 4 }}>
          {financeItems.map(([label, value, unit]) => (
            <Descriptions.Item key={String(label)} label={label}>
              {typeof value === 'number' ? value.toLocaleString() : value} {unit}
            </Descriptions.Item>
          ))}
        </Descriptions>
      </Card>

      {financeMetrics && financeMetrics.tables.length > 0 && (
        <Card title="主要财务指标" extra={<Typography.Text type="secondary">来自 F10「财务分析」文本解析</Typography.Text>}>
          <Tabs
            items={financeMetrics.tables.map((table) => ({
              key: table.title,
              label: table.title,
              children: (
                <Table
                  size="small"
                  rowKey={(row) => row.name}
                  dataSource={table.rows}
                  pagination={false}
                  scroll={{ x: 'max-content' }}
                  columns={[
                    { title: '指标', dataIndex: 'name', fixed: 'left', width: 180 },
                    ...table.periods.map((period, index) => ({
                      title: formatDate(period),
                      key: period,
                      align: 'right' as const,
                      render: (_: unknown, row: { values: string[] }) => row.values[index] || '-',
                    })),
                  ]}
                />
              ),
            }))}
          />
        </Card>
      )}

      <Card
        title={<Space><BarChartOutlined />财务趋势</Space>}
        extra={
          <Space wrap>
            <Radio.Group
              value={financeTrendMode}
              onChange={(event) => setFinanceTrendMode(event.target.value as 'quarter' | 'year')}
              optionType="button"
              buttonStyle="solid"
              options={[
                { label: '按季度', value: 'quarter' },
                { label: '按年度', value: 'year' },
              ]}
            />
            <Segmented<FinanceCompareMode>
              value={financeCompareMode}
              onChange={(value) => setFinanceCompareMode(value)}
              options={[
                { label: '原值', value: 'raw' },
                { label: '同比', value: 'yoy' },
                { label: '环比', value: 'qoq' },
              ]}
            />
            <Segmented<FinanceViewMode>
              value={financeViewMode}
              onChange={(value) => setFinanceViewMode(value)}
              options={[
                { label: '图表', value: 'chart' },
                { label: '表格', value: 'table' },
              ]}
            />
            <Button icon={<FileExcelOutlined />} onClick={handleExportFinanceCsv}>导出数据</Button>
            <Checkbox.Group
              value={selectedFinanceMetrics.filter((metric) => availableFinanceMetrics.some((item) => item.key === metric))}
              options={availableFinanceMetrics.map((metric) => ({ label: metric.label, value: metric.key }))}
              onChange={(values) => setSelectedFinanceMetrics(values as string[])}
            />
          </Space>
        }
      >
        {financeTrendLoading ? (
          <Flex justify="center" align="center" style={{ minHeight: 320 }}><Spin size="large" /></Flex>
        ) : financeTrends && activeFinanceMetrics.length > 0 ? (
          <Space direction="vertical" size={16} style={{ display: 'flex' }}>
            <Row gutter={[16, 16]}>
              {activeFinanceMetrics.slice(0, 4).map((metric) => (
                <Col key={metric.key} xs={24} md={12} xl={6}>
                  <Card size="small">
                    <Statistic
                      title={metric.label}
                      value={typeof latestFinanceRecord?.[metric.key as keyof FinanceTrendRecord] === 'number' ? latestFinanceRecord[metric.key as keyof FinanceTrendRecord] : undefined}
                      formatter={(value: unknown) => formatFinanceMetricValue(
                        typeof value === 'number' ? value : Number(value),
                        { ...metric, axis: financeCompareMode === 'raw' ? metric.axis : 'percent' },
                      )}
                      valueStyle={{ color: metric.color, fontSize: 20 }}
                      suffix={latestFinanceRecord?.label}
                    />
                  </Card>
                </Col>
              ))}
            </Row>
            {financeViewMode === 'chart' ? (
              <Row gutter={[16, 16]}>
                {financeChartGroups.map((group) => {
                  const selectedGroupMetrics = group.metrics.filter((metric: FinanceTrendMetric) => selectedFinanceMetrics.includes(metric.key));
                  const chartMetrics = selectedGroupMetrics.map((metric: FinanceTrendMetric) => ({
                    ...metric,
                    axis: financeCompareMode === 'raw' ? metric.axis : 'percent',
                  }));
                  const chartRef = group.key === 'amount' ? amountChartRef : marginChartRef;
                  return (
                    <Col key={group.key} xs={24}>
                      <Card
                        size="small"
                        title={group.title}
                        extra={
                          <Space>
                            <Typography.Text type="secondary">{group.description}</Typography.Text>
                            <Button size="small" icon={<CameraOutlined />} onClick={() => handleExportFinanceChart(group.key as 'amount' | 'margin')}>
                              导出图片
                            </Button>
                          </Space>
                        }
                      >
                        {chartMetrics.length > 0 ? (
                          <FinanceTrendChart
                            ref={chartRef}
                            records={financeDisplayRecords}
                            metrics={chartMetrics}
                            mode={financeTrendMode}
                          />
                        ) : (
                          <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="请至少选择一个该分组下的指标" />
                        )}
                      </Card>
                    </Col>
                  );
                })}
              </Row>
            ) : (
              <Card size="small" title="财务趋势表格">
                <Table
                  size="small"
                  rowKey={(row) => row.period}
                  columns={financeTableColumns}
                  dataSource={[...financeDisplayRecords].reverse()}
                  pagination={{ pageSize: financeTrendMode === 'year' ? 10 : 12 }}
                  scroll={{ x: 'max-content' }}
                />
              </Card>
            )}
            <Typography.Text type="secondary">
              数据来源于通达信 F10「财务分析」栏目，支持原值、同比、环比视角，以及图表/表格切换和导出。
            </Typography.Text>
          </Space>
        ) : (
          <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="暂无可绘制的财务趋势数据" />
        )}
      </Card>
    </Space>
  );
}

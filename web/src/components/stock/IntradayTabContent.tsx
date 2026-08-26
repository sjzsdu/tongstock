import { useRef } from 'react';
import { Button, Card, Col, Empty, Flex, Row, Space, Spin, Table, Typography } from 'antd';
import type { ColumnsType } from 'antd/es/table';
import type { Finance, MinuteItem, Quote } from '../../types/api';
import MinuteChart from '../charts/MinuteChart';
import { formatShortDate, formatTime } from '../../lib/datetime';

interface IntradayTabContentProps {
  minuteData: MinuteItem[];
  minuteDate: string;
  minuteLoading: boolean;
  minuteError: string;
  quote: Quote;
  finance: Finance | null;
  highlightedIdx: number;
  setHighlightedIdx: (idx: number) => void;
}

export function IntradayTabContent({
  minuteData,
  minuteDate,
  minuteLoading,
  minuteError,
  quote,
  finance,
  highlightedIdx,
  setHighlightedIdx,
}: IntradayTabContentProps) {
  const tradeRowRefs = useRef<Record<number, HTMLDivElement | null>>({});

  if (minuteLoading) {
    return <Card><Flex justify="center" align="center" style={{ minHeight: 240 }}><Spin size="large" /></Flex></Card>;
  }

  if (minuteData.length === 0) {
    return <Card><Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description={minuteError || '暂无可展示的分时数据'} /></Card>;
  }

  const lastClose = quote?.LastClose || 0;
  let totalAmount = 0;
  let totalVolume = 0;
  minuteData.forEach((m) => {
    const vol = Math.abs(m.Number);
    totalAmount += m.Price * vol;
    totalVolume += vol;
  });
  const vwap = totalVolume > 0 ? totalAmount / totalVolume : lastClose;
  const sVol = quote?.SVol || 0;
  const bVol = quote?.BVol || 0;
  const innerVol = Math.abs(sVol);
  const outerVol = Math.abs(bVol);
  const totalVol = innerVol + outerVol;
  const innerPct = totalVol > 0 ? (innerVol / totalVol) * 100 : 50;
  const outerPct = totalVol > 0 ? (outerVol / totalVol) * 100 : 50;
  const turnover = finance && finance.LiuTongGuBen > 0 ? (quote.Volume / finance.LiuTongGuBen) * 100 : 0;

  const minuteColumns: ColumnsType<MinuteItem> = [
    { title: '时间', dataIndex: 'Time', width: 90, render: (value: string) => formatTime(value) },
    {
      title: '价格',
      dataIndex: 'Price',
      align: 'right',
      render: (value: number) => <span className={value >= lastClose ? 'price-up' : 'price-down'}>{value.toFixed(2)}</span>,
    },
    {
      title: '涨跌',
      align: 'right',
      render: (_, row) => {
        const chg = row.Price - lastClose;
        return <span className={chg >= 0 ? 'price-up' : 'price-down'}>{chg > 0 ? '+' : ''}{chg.toFixed(2)}</span>;
      },
    },
    {
      title: '涨幅',
      align: 'right',
      render: (_, row) => {
        const chgPct = lastClose > 0 ? ((row.Price - lastClose) / lastClose) * 100 : 0;
        return <span className={chgPct >= 0 ? 'price-up' : 'price-down'}>{chgPct > 0 ? '+' : ''}{chgPct.toFixed(2)}%</span>;
      },
    },
    { title: '成交量', dataIndex: 'Number', align: 'right', render: (value: number) => `${Math.abs(value).toLocaleString()}手` },
  ];

  const selectedMinute = highlightedIdx >= 0 && highlightedIdx < minuteData.length ? minuteData[highlightedIdx] : null;

  return (
    <Space direction="vertical" size={16} style={{ display: 'flex' }}>
      <Card>
        <Space wrap size={[16, 12]}>
          <Typography.Text type="secondary">{minuteDate || formatShortDate(new Date())}</Typography.Text>
          <Typography.Text>昨收 {lastClose.toFixed(2)}</Typography.Text>
          <Typography.Text>均价 <span className={vwap >= lastClose ? 'price-up' : 'price-down'}>{vwap.toFixed(2)}</span></Typography.Text>
          <Typography.Text>内盘 {(innerVol / 10000).toFixed(0)}万 ({innerPct.toFixed(1)}%)</Typography.Text>
          <Typography.Text>外盘 {(outerVol / 10000).toFixed(0)}万 ({outerPct.toFixed(1)}%)</Typography.Text>
          {turnover > 0 && <Typography.Text>换手 {turnover.toFixed(2)}%</Typography.Text>}
        </Space>
      </Card>
      <Row gutter={[16, 16]}>
        <Col xs={24} xl={15}>
          <Card bodyStyle={{ padding: 0 }}>
            <MinuteChart
              data={minuteData}
              lastClose={lastClose}
              onClickIndex={(idx) => {
                setHighlightedIdx(idx);
                const tableIdx = minuteData.length - 1 - idx;
                const el = tradeRowRefs.current[tableIdx];
                if (el) el.scrollIntoView({ behavior: 'smooth', block: 'center' });
              }}
            />
          </Card>
          {selectedMinute && (
            <Card size="small" style={{ marginTop: 12 }}>
              <Space wrap>
                <Typography.Text>选中: {formatTime(selectedMinute.Time)}</Typography.Text>
                <Typography.Text>价格: {selectedMinute.Price.toFixed(2)}</Typography.Text>
                <Typography.Text>成交量: {Math.abs(selectedMinute.Number).toLocaleString()}手</Typography.Text>
                <Button size="small" onClick={() => setHighlightedIdx(-1)}>清除选择</Button>
              </Space>
            </Card>
          )}
        </Col>
        <Col xs={24} xl={9}>
          <Card title="成交明细">
            <Table
              size="small"
              pagination={{ pageSize: 12 }}
              rowKey={(row, index) => `${row.Time}-${index}`}
              dataSource={[...minuteData].reverse()}
              columns={minuteColumns}
              onRow={(_, rowIndex) => ({
                onClick: () => {
                  if (typeof rowIndex === 'number') setHighlightedIdx(minuteData.length - 1 - rowIndex);
                },
              })}
            />
          </Card>
        </Col>
      </Row>
    </Space>
  );
}

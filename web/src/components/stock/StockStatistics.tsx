import 'react';
import { Card, Col, Row, Statistic } from 'antd';
import { amountWanToYi } from '../../lib/stock-detail';
import type { Quote } from '../../types/api';

interface StockStatisticsProps {
  quote: Quote;
  latestClose: number | undefined;
  valueColor: string;
}

// antd Statistic 按 precision 截断小数串而非四舍五入（40.079999 → "40.07"），
// 传入前先四舍五入到分位
function round2(value: number | undefined): number | undefined {
  if (typeof value !== 'number' || !Number.isFinite(value)) return undefined;
  return Math.round(value * 100) / 100;
}

export function StockStatistics({ quote, latestClose, valueColor }: StockStatisticsProps) {
  const roundedClose = round2(latestClose);
  return (
    <Card>
      <Row gutter={[16, 16]}>
        <Col xs={12} md={8} xl={4}><Statistic title="现价" value={round2(quote.Price)} precision={2} valueStyle={{ color: valueColor }} /></Col>
        <Col xs={12} md={8} xl={4}><Statistic title="涨跌幅" value={round2(((quote.Price - quote.LastClose) / quote.LastClose) * 100)} suffix="%" precision={2} valueStyle={{ color: valueColor }} /></Col>
        <Col xs={12} md={8} xl={4}><Statistic title="开盘" value={round2(quote.Open)} precision={2} /></Col>
        {/* K 线未返回时显示占位符，避免被误读为“收盘 0.00” */}
        <Col xs={12} md={8} xl={4}><Statistic title="收盘" value={roundedClose ?? '--'} precision={roundedClose === undefined ? undefined : 2} valueStyle={{ color: roundedClose ? valueColor : undefined }} /></Col>
        <Col xs={12} md={8} xl={4}><Statistic title="昨收" value={round2(quote.LastClose)} precision={2} /></Col>
        <Col xs={12} md={8} xl={4}><Statistic title="最高" value={round2(quote.High)} precision={2} valueStyle={{ color: '#ef4444' }} /></Col>
        <Col xs={12} md={8} xl={4}><Statistic title="最低" value={round2(quote.Low)} precision={2} valueStyle={{ color: '#22c55e' }} /></Col>
        {/* Volume 单位为手；先取整，antd precision=0 是截断不是四舍五入 */}
        <Col xs={12} md={8} xl={4}><Statistic title="成交量" value={Math.round(quote.Volume / 10000)} suffix="万手" precision={0} /></Col>
        <Col xs={12} md={8} xl={4}><Statistic title="成交额" value={round2(amountWanToYi(quote.Amount))} suffix="亿" precision={2} /></Col>
      </Row>
    </Card>
  );
}

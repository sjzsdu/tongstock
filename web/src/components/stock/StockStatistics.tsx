import { useEffect } from 'react';
import { Card, Col, Row, Statistic } from 'antd';
import { amountWanToYi } from '../../lib/stock-detail';

// Keep react import for JSX runtime
useEffect(() => {}, []);

interface StockStatisticsProps {
  quote: any;
  latestClose: number | undefined;
  valueColor: string;
}

export function StockStatistics({ quote, latestClose, valueColor }: StockStatisticsProps) {
  return (
    <Card>
      <Row gutter={[16, 16]}>
        <Col xs={12} md={8} xl={4}><Statistic title="现价" value={quote.Price} precision={2} valueStyle={{ color: valueColor }} /></Col>
        <Col xs={12} md={8} xl={4}><Statistic title="涨跌幅" value={((quote.Price - quote.LastClose) / quote.LastClose) * 100} suffix="%" precision={2} valueStyle={{ color: valueColor }} /></Col>
        <Col xs={12} md={8} xl={4}><Statistic title="开盘" value={quote.Open} precision={2} /></Col>
        <Col xs={12} md={8} xl={4}><Statistic title="收盘" value={latestClose} precision={2} valueStyle={{ color: latestClose ? valueColor : undefined }} /></Col>
        <Col xs={12} md={8} xl={4}><Statistic title="昨收" value={quote.LastClose} precision={2} /></Col>
        <Col xs={12} md={8} xl={4}><Statistic title="最高" value={quote.High} precision={2} valueStyle={{ color: '#ef4444' }} /></Col>
        <Col xs={12} md={8} xl={4}><Statistic title="最低" value={quote.Low} precision={2} valueStyle={{ color: '#22c55e' }} /></Col>
        <Col xs={12} md={8} xl={4}><Statistic title="成交量" value={quote.Volume / 10000} suffix="万" precision={0} /></Col>
        <Col xs={12} md={8} xl={4}><Statistic title="成交额" value={amountWanToYi(quote.Amount)} suffix="亿" precision={2} /></Col>
      </Row>
    </Card>
  );
}

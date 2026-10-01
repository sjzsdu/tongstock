import 'react';
import { Button, Card, Col, Empty, List, Row, Space, Spin } from 'antd';
import { InfoCircleOutlined } from '@ant-design/icons';
import type { CompanyCategory } from '../../types/api';
import TabContent from '../TabContent';
import TdxContent from '../TdxContent';
import { parseTdxText } from '../../lib/tdx-parser';

interface CompanyTabContentProps {
  companyCats: CompanyCategory[];
  companyContent: string;
  selectedCat: string;
  companyLoading?: boolean;
  loadCompanyContent: (cat: string | CompanyCategory) => Promise<void>;
}

export function CompanyTabContent({ companyCats, companyContent, selectedCat, companyLoading, loadCompanyContent }: CompanyTabContentProps) {
  return (
    <TabContent>
      <Row gutter={[16, 16]}>
        <Col xs={24} lg={6}>
          <Card title="F10目录">
            {companyLoading && companyCats.length === 0 ? (
              <div style={{ textAlign: 'center', padding: 24 }}><Spin /></div>
            ) : (
              <List
                size="small"
                dataSource={companyCats}
                locale={{ emptyText: '暂无F10目录' }}
                renderItem={(cat) => (
                  <List.Item>
                    <Button type={selectedCat === cat.Name ? 'primary' : 'text'} block onClick={() => void loadCompanyContent(cat)}>
                      {cat.Name}
                    </Button>
                  </List.Item>
                )}
              />
            )}
          </Card>
        </Col>
        <Col xs={24} lg={18}>
          <Card title={<Space><InfoCircleOutlined />内容</Space>}>
            {companyContent ? (
              <TdxContent sections={parseTdxText(companyContent)} />
            ) : (
              <Empty description="点击左侧目录查看内容" />
            )}
          </Card>
        </Col>
      </Row>
    </TabContent>
  );
}

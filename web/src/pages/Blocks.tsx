import { useEffect, useState } from 'react';
import { useNavigate } from 'react-router-dom';
import {
  ArrowRightOutlined,
  FundOutlined,
  StockOutlined,
} from '@ant-design/icons';
import {
  Button,
  Card,
  Col,
  Empty,
  List,
  Row,
  Select,
  Skeleton,
  Space,
  Table,
  Tag,
  Typography,
} from 'antd';
import type { ColumnsType } from 'antd/es/table';
import { api } from '../api/client';

interface BlockInfo {
  name: string;
  type: number;
  count: number;
}

interface BlockStock {
  code: string;
  name: string;
  exchange: string;
}

export default function Blocks() {
  const navigate = useNavigate();
  const [files, setFiles] = useState<{ file: string; name: string; desc: string }[]>([]);
  const [selectedFile, setSelectedFile] = useState('block_fg.dat');
  const [blocks, setBlocks] = useState<BlockInfo[]>([]);
  const [selectedBlock, setSelectedBlock] = useState<string | null>(null);
  const [stocks, setStocks] = useState<BlockStock[]>([]);
  const [loadingFiles, setLoadingFiles] = useState(true);
  const [loadingBlocks, setLoadingBlocks] = useState(false);
  const [loadingStocks, setLoadingStocks] = useState(false);

  useEffect(() => {
    void loadFiles();
  }, []);

  useEffect(() => {
    if (selectedFile) {
      void loadBlocks();
    }
  }, [selectedFile]);

  useEffect(() => {
    if (selectedBlock) {
      void loadStocks();
    }
  }, [selectedBlock]);

  const loadFiles = async () => {
    setLoadingFiles(true);
    try {
      const result = await api.blockFiles();
      setFiles(result.files);
      if (result.files.length > 0) {
        setSelectedFile(result.files[0].file);
      }
    } finally {
      setLoadingFiles(false);
    }
  };

  const loadBlocks = async () => {
    setLoadingBlocks(true);
    setSelectedBlock(null);
    setStocks([]);
    try {
      const result = await api.blockList(selectedFile);
      setBlocks(result.blocks);
    } finally {
      setLoadingBlocks(false);
    }
  };

  const loadStocks = async () => {
    setLoadingStocks(true);
    try {
      const result = await api.blockShow(selectedBlock ?? undefined, undefined, selectedFile);
      setStocks(result.stocks ?? []);
    } finally {
      setLoadingStocks(false);
    }
  };

  const columns: ColumnsType<BlockStock> = [
    {
      title: '代码',
      dataIndex: 'code',
      width: 120,
      render: (code: string) => (
        <Button type="link" size="small" onClick={() => navigate(`/stock/${code}`)}>
          {code}
        </Button>
      ),
    },
    {
      title: '名称',
      dataIndex: 'name',
      width: 150,
    },
    {
      title: '交易所',
      dataIndex: 'exchange',
      width: 80,
      render: (ex: string) => <Tag color={ex === 'sh' ? 'blue' : 'green'}>{ex.toUpperCase()}</Tag>,
    },
    {
      title: '操作',
      width: 80,
      render: (_, record) => (
        <Button type="link" size="small" icon={<ArrowRightOutlined />} onClick={() => navigate(`/stock/${record.code}`)} />
      ),
    },
  ];

  return (
    <Space direction="vertical" size={24} style={{ display: 'flex' }}>
      <Card bordered={false} style={{ background: 'linear-gradient(135deg, rgba(22,119,255,0.22), rgba(14,165,233,0.12))' }}>
        <Space direction="vertical" size={10} style={{ display: 'flex' }}>
          <Tag color="blue" style={{ width: 'fit-content', marginInlineEnd: 0 }}>板块热点</Tag>
          <Typography.Title level={2} style={{ margin: 0 }}>
            板块与热点
          </Typography.Title>
          <Typography.Text type="secondary">
            查看行业板块、概念板块的成分股列表，把握市场热点方向。
          </Typography.Text>
        </Space>
      </Card>

      <Row gutter={[16, 16]}>
        <Col xs={24} lg={8}>
          <Card
            title={
              <Space>
                <FundOutlined />
                <span>板块文件</span>
              </Space>
            }
          >
            {loadingFiles ? (
              <Skeleton active paragraph={{ rows: 2 }} title={false} />
            ) : files.length === 0 ? (
              <Empty description="暂无板块数据" image={Empty.PRESENTED_IMAGE_SIMPLE} />
            ) : (
              <Space direction="vertical" size={12} style={{ display: 'flex' }}>
                <Select
                  value={selectedFile}
                  onChange={setSelectedFile}
                  options={files.map((f) => ({ value: f.file, label: `${f.name} (${f.desc})` }))}
                  style={{ width: '100%' }}
                />
              </Space>
            )}
          </Card>
        </Col>

        <Col xs={24} lg={8}>
          <Card
            title={
              <Space>
                <StockOutlined />
                <span>板块列表</span>
              </Space>
            }
          >
            {loadingBlocks ? (
              <Skeleton active paragraph={{ rows: 6 }} title={false} />
            ) : blocks.length === 0 ? (
              <Empty description="暂无板块" image={Empty.PRESENTED_IMAGE_SIMPLE} />
            ) : (
              <List
                dataSource={blocks}
                renderItem={(item) => (
                  <List.Item
                    onClick={() => setSelectedBlock(item.name)}
                    style={{ cursor: 'pointer', background: selectedBlock === item.name ? 'rgba(22,119,255,0.1)' : undefined }}
                  >
                    <List.Item.Meta
                      title={item.name}
                      description={`${item.count} 只股票`}
                    />
                  </List.Item>
                )}
                style={{ maxHeight: 400, overflow: 'auto' }}
              />
            )}
          </Card>
        </Col>

        <Col xs={24} lg={8}>
          <Card
            title={
              <Space>
                <StockOutlined />
                <span>成分股</span>
              </Space>
            }
          >
            {loadingStocks ? (
              <Skeleton active paragraph={{ rows: 6 }} title={false} />
            ) : !selectedBlock ? (
              <Empty description="请先选择板块" image={Empty.PRESENTED_IMAGE_SIMPLE} />
            ) : stocks.length === 0 ? (
              <Empty description="该板块暂无成分股" image={Empty.PRESENTED_IMAGE_SIMPLE} />
            ) : (
              <Table
                columns={columns}
                dataSource={stocks}
                rowKey="code"
                size="small"
                pagination={{ pageSize: 10, size: 'small' }}
              />
            )}
          </Card>
        </Col>
      </Row>
    </Space>
  );
}
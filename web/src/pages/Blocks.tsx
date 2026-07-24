import { useEffect, useState, useMemo } from 'react';
import { useNavigate } from 'react-router-dom';
import {
  ArrowRightOutlined,
  FundOutlined,
  PlusOutlined,
  StockOutlined,
  DeleteOutlined,
  EditOutlined,
  FilterOutlined,
  SearchOutlined,
  SaveOutlined,
  CheckOutlined,
  CloseOutlined,
} from '@ant-design/icons';
import {
  Button,
  Card,
  Col,
  Divider,
  Empty,
  Flex,
  Form,
  Input,
  InputNumber,
  List,
  Modal,
  Pagination,
  Row,
  Select,
  Skeleton,
  Space,
  Table,
  Tag,
  Tabs,
  Typography,
  Alert,
  Switch,
  message,
} from 'antd';
import type { ColumnsType } from 'antd/es/table';
import { api } from '../api/client';
import type { CustomStockPool, StockPoolFilter, MarketCodeItem, FilterField } from '../types/api';

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

// Filter configuration options
const filterFieldOptions: { field: FilterField; label: string; type: 'range' | 'select' | 'boolean'; unit?: string }[] = [
  { field: 'marketCap', label: '流通市值', type: 'range', unit: '亿' },
  { field: 'price', label: '股价', type: 'range', unit: '元' },
  { field: 'turnoverRate', label: '换手率', type: 'range', unit: '%' },
  { field: 'changePct', label: '涨跌幅', type: 'range', unit: '%' },
  { field: 'volumeRatio', label: '量比', type: 'range' },
  { field: 'exchange', label: '交易所', type: 'select' },
  { field: 'board', label: '板块类型', type: 'select' },
  { field: 'excludeST', label: '排除ST', type: 'boolean' },
];

const exchangeOptions = [
  { value: 'sh', label: '上海(SH)' },
  { value: 'sz', label: '深圳(SZ)' },
  { value: 'bj', label: '北京(BJ)' },
];

const boardOptions = [
  { value: 'main', label: '主板' },
  { value: 'chuangye', label: '创业板' },
  { value: 'kechuang', label: '科创板' },
  { value: 'beijiao', label: '北交所' },
];

export default function Blocks() {
  const navigate = useNavigate();

  // === TDX Block Tab State ===
  const [files, setFiles] = useState<{ file: string; name: string; desc: string }[]>([]);
  const [selectedFile, setSelectedFile] = useState('block_fg.dat');
  const [blocks, setBlocks] = useState<BlockInfo[]>([]);
  const [selectedBlock, setSelectedBlock] = useState<string | null>(null);
  const [blockStocks, setBlockStocks] = useState<BlockStock[]>([]);
  const [loadingFiles, setLoadingFiles] = useState(true);
  const [loadingBlocks, setLoadingBlocks] = useState(false);
  const [loadingBlockStocks, setLoadingBlockStocks] = useState(false);

  // === Custom Pool Tab State ===
  const [customPools, setCustomPools] = useState<CustomStockPool[]>([]);
  const [currentPoolId, setCurrentPoolId] = useState('');
  const [allMarketCodes, setAllMarketCodes] = useState<MarketCodeItem[]>([]);
  const [poolPage, setPoolPage] = useState(1);
  const [poolPageSize] = useState(20);

  const [stockCache, setStockCache] = useState<Record<string, { codes: MarketCodeItem[]; timestamp: number }>>({});
  const [cacheDuration] = useState(5 * 60 * 1000); // 5 minutes cache

  // === All Stocks Tab State ===
  interface StockInfoItem {
    code: string;
    name: string;
    exchange: string;
    price: number;
    marketCap: number;
    turnoverRate: number;
    changePct: number;
    volumeRatio: number;
  }
  
  const [allStocks, setAllStocks] = useState<StockInfoItem[]>([]);
  const [allStocksPage, setAllStocksPage] = useState(1);
  const [allStocksPageSize] = useState(20);
  const [allStocksLoading, setAllStocksLoading] = useState(false);
  const [allStocksTotal, setAllStocksTotal] = useState(0);
  
  // Filters for all stocks
  const [allStocksFilters, setAllStocksFilters] = useState({
    minMarketCap: undefined as number | undefined,
    maxMarketCap: undefined as number | undefined,
    minPrice: undefined as number | undefined,
    maxPrice: undefined as number | undefined,
    minTurnoverRate: undefined as number | undefined,
    maxTurnoverRate: undefined as number | undefined,
    exchange: '' as string,
    excludeST: false,
  });
  const [showSaveAsPoolModal, setShowSaveAsPoolModal] = useState(false);
  const [saveAsPoolName, setSaveAsPoolName] = useState('');

  // === Modal States ===
  const [showAddModal, setShowAddModal] = useState(false);
  const [showEditModal, setShowEditModal] = useState(false);
  const [editingPool, setEditingPool] = useState<CustomStockPool | null>(null);
  const [poolForm] = Form.useForm();
  const [currentFilters, setCurrentFilters] = useState<StockPoolFilter[]>([]);

  // Load stock pools from backend
  useEffect(() => {
    const loadPools = async () => {
      try {
        const result = await api.stockpoolList();
        let pools = result.pools;
        // If no pools exist, create a default one
        if (pools.length === 0) {
          const defaultPool: CustomStockPool = {
            id: 'default',
            name: '杨永兴池(50-200亿)',
            description: '适合杨永兴策略的市值范围',
            filters: [{ field: 'marketCap', operator: 'between', value: [50, 200] }],
            createdAt: new Date().toISOString(),
            updatedAt: new Date().toISOString(),
          };
          await api.stockpoolUpsert(defaultPool);
          pools = [defaultPool];
        }
        setCustomPools(pools);
        if (pools.length > 0) {
          setCurrentPoolId(pools[0].id);
        }
      } catch {
        // Fallback to default pool if API fails
        const defaultPool: CustomStockPool = {
          id: 'default',
          name: '杨永兴池(50-200亿)',
          description: '适合杨永兴策略的市值范围',
          filters: [{ field: 'marketCap', operator: 'between', value: [50, 200] }],
          createdAt: new Date().toISOString(),
          updatedAt: new Date().toISOString(),
        };
        setCustomPools([defaultPool]);
        setCurrentPoolId('default');
      } finally {
        // Loading complete
      }
    };
    void loadPools();
  }, []);

  // === TDX Block Functions ===
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
    setBlockStocks([]);
    try {
      const result = await api.blockList(selectedFile);
      setBlocks(result.blocks);
    } finally {
      setLoadingBlocks(false);
    }
  };

  const loadBlockStocks = async () => {
    setLoadingBlockStocks(true);
    try {
      const result = await api.blockShow(selectedBlock ?? undefined, undefined, selectedFile);
      setBlockStocks(result.stocks ?? []);
    } finally {
      setLoadingBlockStocks(false);
    }
  };

  // === Custom Pool Functions ===
  const currentPool = useMemo(() => customPools.find(p => p.id === currentPoolId), [customPools, currentPoolId]);

  const applyFilter = (codes: MarketCodeItem[], filter: StockPoolFilter): MarketCodeItem[] => {
    switch (filter.field) {
      case 'exchange':
        if (filter.operator === 'in') {
          return codes.filter(c => filter.value.includes(c.exchange));
        }
        break;
      case 'board':
        return codes.filter(c => {
          const code = c.code;
          let matched = false;
          if (filter.value.includes('main') && (code.startsWith('000') || code.startsWith('002') || code.startsWith('600') || code.startsWith('601'))) matched = true;
          if (filter.value.includes('chuangye') && (code.startsWith('300') || code.startsWith('301'))) matched = true;
          if (filter.value.includes('kechuang') && code.startsWith('688')) matched = true;
          if (filter.value.includes('beijiao') && code.startsWith('8')) matched = true;
          return matched;
        });
      case 'excludeST':
        if (filter.value[0] === true) {
          return codes.filter(c => !c.name.includes('ST') && !c.name.includes('*ST'));
        }
        break;
    }
    return codes;
  };

  // Generate cache key based on pool filters
  const getCacheKey = (pool: CustomStockPool): string => {
    return pool.id + '-' + JSON.stringify(pool.filters);
  };

  // Load market codes based on current pool filters with caching
  useEffect(() => {
    if (!currentPool) {
      setAllMarketCodes([]);
      return;
    }

    const cacheKey = getCacheKey(currentPool);
    const cached = stockCache[cacheKey];
    
    // Use cache if available and not expired
    if (cached && Date.now() - cached.timestamp < cacheDuration) {
      setAllMarketCodes(cached.codes);
      return;
    }

    let mounted = true;
    
    const loadCodes = async () => {
      try {
        // Check if there are market cap filters
        const marketCapFilter = currentPool.filters.find((f) => f.field === 'marketCap');
        
        let codes: MarketCodeItem[] = [];
        if (marketCapFilter && marketCapFilter.operator === 'between') {
          const minCap = marketCapFilter.value[0] as number;
          const maxCap = marketCapFilter.value[1] as number;
          const result = await api.codesWithMarketCap(minCap, maxCap);
          if (result.codes) {
            codes = result.codes;
          }
        } else {
          const result = await api.codesMarket();
          if (result.codes) {
            codes = result.codes;
          }
        }
        
        if (mounted) {
          setAllMarketCodes(codes);
          // Cache the result
          setStockCache((prev) => ({
            ...prev,
            [cacheKey]: { codes, timestamp: Date.now() },
          }));
        }
      } catch {
        setAllMarketCodes([]);
      }
    };
    void loadCodes();
    return () => { mounted = false; };
  }, [currentPool]);

  // Apply non-market-cap filters to market codes
  const filteredPoolStocks = useMemo(() => {
    if (!currentPool || allMarketCodes.length === 0) return [];
    
    let filtered = [...allMarketCodes];
    for (const filter of currentPool.filters) {
      // Skip marketCap filter since it's already applied by backend
      if (filter.field === 'marketCap') continue;
      filtered = applyFilter(filtered, filter);
    }
    return filtered;
  }, [currentPool, allMarketCodes]);

  // When pool changes, reset page
  useEffect(() => {
    setPoolPage(1);
  }, [currentPoolId]);

  const addCustomPool = () => {
    poolForm.resetFields();
    setCurrentFilters([]);
    setShowAddModal(true);
  };

  const editCustomPool = (pool: CustomStockPool) => {
    setEditingPool(pool);
    poolForm.setFieldsValue({ name: pool.name, description: pool.description || '' });
    setCurrentFilters([...pool.filters]);
    setShowEditModal(true);
  };

  const deleteCustomPool = (poolId: string) => {
    if (customPools.length <= 1) {
      Modal.warning({ title: '提示', content: '至少保留一个股票池' });
      return;
    }
    Modal.confirm({
      title: '确认删除',
      content: '确定要删除这个股票池吗？',
      okText: '删除',
      okType: 'danger',
      onOk: async () => {
        try {
          await api.stockpoolDelete(poolId);
          const newPools = customPools.filter(p => p.id !== poolId);
          setCustomPools(newPools);
          if (currentPoolId === poolId) {
            setCurrentPoolId(newPools[0]?.id || '');
          }
        } catch {
          message.error('删除失败');
        }
      },
    });
  };

  const savePool = async () => {
    const values = poolForm.getFieldsValue();
    const now = new Date().toISOString();
    try {
      if (editingPool) {
        const updatedPool: CustomStockPool = {
          ...editingPool,
          name: values.name,
          description: values.description,
          filters: currentFilters,
          updatedAt: now,
        };
        await api.stockpoolUpsert(updatedPool);
        setCustomPools(prev => prev.map(p => p.id === editingPool.id ? updatedPool : p));
        setShowEditModal(false);
      } else {
        const newPool: CustomStockPool = {
          id: Date.now().toString(),
          name: values.name,
          description: values.description,
          filters: currentFilters,
          createdAt: now,
          updatedAt: now,
        };
        await api.stockpoolUpsert(newPool);
        setCustomPools(prev => [...prev, newPool]);
        setCurrentPoolId(newPool.id);
        setShowAddModal(false);
      }
    } catch {
      message.error('保存失败');
    }
  };

  const addFilter = () => {
    setCurrentFilters(prev => [...prev, { field: 'marketCap', operator: 'between', value: [0, 0] }]);
  };

  const removeFilter = (index: number) => {
    setCurrentFilters(prev => prev.filter((_, i) => i !== index));
  };

  const updateFilter = (index: number, field: keyof StockPoolFilter, value: unknown) => {
    setCurrentFilters(prev => prev.map((f, i) => i === index ? { ...f, [field]: value } : f));
  };

  const updateFilterValue = (index: number, idx: number, value: number | string | boolean) => {
    setCurrentFilters(prev => prev.map((f, i) => {
      if (i === index) {
        const newValues = [...f.value];
        newValues[idx] = value;
        return { ...f, value: newValues };
      }
      return f;
    }));
  };

  // === All Stocks Functions ===
  const loadAllStocks = async () => {
    setAllStocksLoading(true);
    try {
      const result = await api.stockinfoList(
        allStocksFilters.minMarketCap,
        allStocksFilters.maxMarketCap,
        allStocksFilters.exchange || undefined
      );
      let stocks = result.infos || [];
      
      // Apply client-side filters
      if (allStocksFilters.minPrice != null) {
        stocks = stocks.filter(s => s.price >= allStocksFilters.minPrice!);
      }
      if (allStocksFilters.maxPrice != null) {
        stocks = stocks.filter(s => s.price <= allStocksFilters.maxPrice!);
      }
      if (allStocksFilters.minTurnoverRate != null) {
        stocks = stocks.filter(s => s.turnoverRate >= allStocksFilters.minTurnoverRate!);
      }
      if (allStocksFilters.maxTurnoverRate != null) {
        stocks = stocks.filter(s => s.turnoverRate <= allStocksFilters.maxTurnoverRate!);
      }
      if (allStocksFilters.excludeST) {
        stocks = stocks.filter(s => !s.name.includes('ST') && !s.name.includes('*ST'));
      }
      
      setAllStocks(stocks);
      setAllStocksTotal(stocks.length);
      setAllStocksPage(1);
    } catch {
      setAllStocks([]);
      setAllStocksTotal(0);
    } finally {
      setAllStocksLoading(false);
    }
  };

  const handleAllStocksFilterChange = (key: keyof typeof allStocksFilters, value: number | string | boolean | undefined) => {
    setAllStocksFilters(prev => ({ ...prev, [key]: value }));
  };

  const resetAllStocksFilters = () => {
    setAllStocksFilters({
      minMarketCap: undefined,
      maxMarketCap: undefined,
      minPrice: undefined,
      maxPrice: undefined,
      minTurnoverRate: undefined,
      maxTurnoverRate: undefined,
      exchange: '',
      excludeST: false,
    });
  };

  const saveAsStockPool = async () => {
    if (!saveAsPoolName.trim()) {
      message.error('请输入股票池名称');
      return;
    }
    
    const filters: StockPoolFilter[] = [];
    
    if (allStocksFilters.minMarketCap != null || allStocksFilters.maxMarketCap != null) {
      filters.push({
        field: 'marketCap',
        operator: 'between',
        value: [allStocksFilters.minMarketCap || 0, allStocksFilters.maxMarketCap || 999999],
      });
    }
    if (allStocksFilters.exchange) {
      filters.push({
        field: 'exchange',
        operator: 'in',
        value: [allStocksFilters.exchange],
      });
    }
    if (allStocksFilters.excludeST) {
      filters.push({
        field: 'excludeST',
        operator: 'between',
        value: [true],
      });
    }
    
    const newPool: CustomStockPool = {
      id: Date.now().toString(),
      name: saveAsPoolName.trim(),
      description: `筛选条件: ${filters.length > 0 ? filters.map(f => f.field).join(', ') : '无'}`,
      filters,
      createdAt: new Date().toISOString(),
      updatedAt: new Date().toISOString(),
    };
    
    try {
      await api.stockpoolUpsert(newPool);
      setCustomPools(prev => [...prev, newPool]);
      setShowSaveAsPoolModal(false);
      setSaveAsPoolName('');
      message.success('股票池保存成功');
    } catch {
      message.error('保存失败');
    }
  };

  const paginatedAllStocks = useMemo(() => {
    const start = (allStocksPage - 1) * allStocksPageSize;
    return allStocks.slice(start, start + allStocksPageSize);
  }, [allStocks, allStocksPage, allStocksPageSize]);

  // === Effects ===
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
      void loadBlockStocks();
    }
  }, [selectedBlock]);

  useEffect(() => {
    void loadAllStocks();
  }, [allStocksFilters]);

  // === Columns ===
  const blockStockColumns: ColumnsType<BlockStock> = [
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
      render: (ex: string) => <Tag color={ex === 'sh' ? 'blue' : ex === 'sz' ? 'green' : 'orange'}>{ex.toUpperCase()}</Tag>,
    },
    {
      title: '操作',
      width: 80,
      render: (_, record) => (
        <Button type="link" size="small" icon={<ArrowRightOutlined />} onClick={() => navigate(`/stock/${record.code}`)} />
      ),
    },
  ];

  const poolStockColumns: ColumnsType<MarketCodeItem> = [
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
      render: (ex: string) => <Tag color={ex === 'sh' ? 'blue' : ex === 'sz' ? 'green' : 'orange'}>{ex.toUpperCase()}</Tag>,
    },
    {
      title: '操作',
      width: 80,
      render: (_, record) => (
        <Button type="link" size="small" icon={<ArrowRightOutlined />} onClick={() => navigate(`/stock/${record.code}`)} />
      ),
    },
  ];

  // All Stocks columns
  const allStockColumns: ColumnsType<StockInfoItem> = [
    {
      title: '代码',
      dataIndex: 'code',
      width: 100,
      render: (code: string) => (
        <Button type="link" size="small" onClick={() => navigate(`/stock/${code}`)}>
          {code}
        </Button>
      ),
    },
    {
      title: '名称',
      dataIndex: 'name',
      width: 120,
    },
    {
      title: '交易所',
      dataIndex: 'exchange',
      width: 80,
      render: (ex: string) => <Tag color={ex === 'sh' ? 'blue' : ex === 'sz' ? 'green' : 'orange'}>{ex.toUpperCase()}</Tag>,
    },
    {
      title: '股价(元)',
      dataIndex: 'price',
      width: 100,
      render: (price: number) => price.toFixed(2),
    },
    {
      title: '流通市值(亿)',
      dataIndex: 'marketCap',
      width: 120,
      render: (cap: number) => cap.toFixed(2),
    },
    {
      title: '涨跌幅(%)',
      dataIndex: 'changePct',
      width: 100,
      render: (pct: number) => (
        <Typography.Text type={pct >= 0 ? 'success' : 'danger'}>
          {pct >= 0 ? '+' : ''}{pct.toFixed(2)}
        </Typography.Text>
      ),
    },
    {
      title: '换手率(%)',
      dataIndex: 'turnoverRate',
      width: 100,
      render: (rate: number) => rate.toFixed(2),
    },
    {
      title: '量比',
      dataIndex: 'volumeRatio',
      width: 80,
      render: (ratio: number) => ratio.toFixed(2),
    },
    {
      title: '操作',
      width: 80,
      render: (_, record) => (
        <Button type="link" size="small" icon={<ArrowRightOutlined />} onClick={() => navigate(`/stock/${record.code}`)} />
      ),
    },
  ];

  // === Filter Rendering ===
  const renderFilterField = (filter: StockPoolFilter, index: number) => {
    const fieldOption = filterFieldOptions.find(f => f.field === filter.field);
    if (!fieldOption) return null;

    switch (fieldOption.type) {
      case 'range':
        // 根据操作符决定渲染方式
        switch (filter.operator) {
          case 'between':
            return (
              <Space key={index} className="filter-range">
                <InputNumber
                  min={fieldOption.field === 'changePct' ? -100 : 0}
                  style={{ width: 80 }}
                  value={filter.value[0] as number}
                  onChange={(v) => updateFilterValue(index, 0, v || 0)}
                  placeholder="最小"
                />
                <span className="filter-separator">~</span>
                <InputNumber
                  min={0}
                  style={{ width: 80 }}
                  value={filter.value[1] as number}
                  onChange={(v) => updateFilterValue(index, 1, v || 0)}
                  placeholder="最大"
                />
                {fieldOption.unit && <span className="filter-unit">{fieldOption.unit}</span>}
              </Space>
            );
          case 'gt':
          case 'gte':
            return (
              <Space key={index} className="filter-range">
                <InputNumber
                  min={fieldOption.field === 'changePct' ? -100 : 0}
                  style={{ width: 120 }}
                  value={filter.value[0] as number}
                  onChange={(v) => updateFilterValue(index, 0, v || 0)}
                  placeholder={filter.operator === 'gt' ? '大于' : '大于等于'}
                />
                {fieldOption.unit && <span className="filter-unit">{fieldOption.unit}</span>}
              </Space>
            );
          case 'lt':
          case 'lte':
            return (
              <Space key={index} className="filter-range">
                <InputNumber
                  min={fieldOption.field === 'changePct' ? -100 : 0}
                  style={{ width: 120 }}
                  value={filter.value[0] as number}
                  onChange={(v) => updateFilterValue(index, 0, v || 0)}
                  placeholder={filter.operator === 'lt' ? '小于' : '小于等于'}
                />
                {fieldOption.unit && <span className="filter-unit">{fieldOption.unit}</span>}
              </Space>
            );
          default:
            return null;
        }
      case 'select':
        if (filter.field === 'exchange') {
          return (
            <Select
              key={index}
              mode="multiple"
              style={{ width: 180 }}
              options={exchangeOptions}
              value={filter.value as string[]}
              onChange={(v) => updateFilter(index, 'value', v)}
              placeholder="选择交易所"
              size="small"
            />
          );
        }
        if (filter.field === 'board') {
          return (
            <Select
              key={index}
              mode="multiple"
              style={{ width: 180 }}
              options={boardOptions}
              value={filter.value as string[]}
              onChange={(v) => updateFilter(index, 'value', v)}
              placeholder="选择板块类型"
              size="small"
            />
          );
        }
        break;
      case 'boolean':
        return (
          <Switch
            key={index}
            checked={filter.value[0] === true}
            onChange={(v) => updateFilterValue(index, 0, v)}
            checkedChildren={<CheckOutlined />}
            unCheckedChildren={<CloseOutlined />}
          />
        );
    }
    return null;
  };

  const renderFilterLabel = (filter: StockPoolFilter) => {
    const fieldOption = filterFieldOptions.find(f => f.field === filter.field);
    if (!fieldOption) return filter.field;
    
    if (fieldOption.type === 'boolean') {
      return <>{fieldOption.label}</>;
    }
    if (filter.operator === 'between') {
      return `${fieldOption.label} ${filter.value[0]}~${filter.value[1]}${fieldOption.unit || ''}`;
    }
    return `${fieldOption.label} ${filter.value.join(', ')}`;
  };

  const paginatedPoolStocks = useMemo(() => {
    const start = (poolPage - 1) * poolPageSize;
    return filteredPoolStocks.slice(start, start + poolPageSize);
  }, [filteredPoolStocks, poolPage, poolPageSize]);

  return (
    <Space direction="vertical" size={24} style={{ display: 'flex' }}>
      <Card bordered={false} style={{ background: 'linear-gradient(135deg, rgba(22,119,255,0.22), rgba(14,165,233,0.12))' }}>
        <Space direction="vertical" size={10} style={{ display: 'flex' }}>
          <Tag color="blue" style={{ width: 'fit-content', marginInlineEnd: 0 }}>股票池管理</Tag>
          <Typography.Title level={2} style={{ margin: 0 }}>
            板块与股票池
          </Typography.Title>
          <Typography.Text type="secondary">
            管理TDX板块和自定义股票池，支持多种筛选条件组合。
          </Typography.Text>
        </Space>
      </Card>

      <Tabs
        defaultActiveKey="blocks"
        items={[
          {
            key: 'blocks',
            label: (
              <Space>
                <FundOutlined />
                <span>TDX 板块</span>
              </Space>
            ),
            children: (
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
                    {loadingBlockStocks ? (
                      <Skeleton active paragraph={{ rows: 6 }} title={false} />
                    ) : !selectedBlock ? (
                      <Empty description="请先选择板块" image={Empty.PRESENTED_IMAGE_SIMPLE} />
                    ) : blockStocks.length === 0 ? (
                      <Empty description="该板块暂无成分股" image={Empty.PRESENTED_IMAGE_SIMPLE} />
                    ) : (
                      <Table
                        columns={blockStockColumns}
                        dataSource={blockStocks}
                        rowKey="code"
                        size="small"
                        pagination={{ pageSize: 10, size: 'small' }}
                      />
                    )}
                  </Card>
                </Col>
              </Row>
            ),
          },
          {
            key: 'custom',
            label: (
              <Space>
                <StockOutlined />
                <span>自定义股票池</span>
              </Space>
            ),
            children: (
              <Row gutter={[16, 16]}>
                {/* Left: Pool List */}
                <Col xs={24} lg={5}>
                  <Card
                    title={
                      <Space>
                        <StockOutlined />
                        <span>股票池列表</span>
                        <Button type="primary" size="small" icon={<PlusOutlined />} onClick={addCustomPool} />
                      </Space>
                    }
                    bodyStyle={{ padding: '12px' }}
                  >
                    <List
                      dataSource={customPools}
                      renderItem={(item) => (
                        <List.Item
                          onClick={() => { setCurrentPoolId(item.id); setPoolPage(1); }}
                          style={{ 
                            cursor: 'pointer', 
                            background: currentPoolId === item.id ? 'rgba(22,119,255,0.1)' : undefined,
                            marginBottom: '8px',
                            borderRadius: '6px',
                            padding: '8px',
                          }}
                          extra={
                            <Space size={4}>
                              <Button size="small" icon={<EditOutlined />} onClick={(e) => { e.stopPropagation(); editCustomPool(item); }} />
                              <Button size="small" danger icon={<DeleteOutlined />} onClick={(e) => { e.stopPropagation(); deleteCustomPool(item.id); }} disabled={customPools.length <= 1} />
                            </Space>
                          }
                        >
                          <List.Item.Meta
                            title={<Typography.Text strong>{item.name}</Typography.Text>}
                            description={item.description || `${item.filters.length} 个筛选条件`}
                          />
                        </List.Item>
                      )}
                      style={{ maxHeight: 600, overflow: 'auto' }}
                    />
                  </Card>
                </Col>

                {/* Right: Filters + Stocks */}
                <Col xs={24} lg={19}>
                  {currentPool ? (
                    <Space direction="vertical" size={16} style={{ display: 'flex', width: '100%' }}>
                      {/* Filter Panel */}
                      <Card
                        title={
                          <Space>
                            <FilterOutlined />
                            <span>筛选条件</span>
                            <Typography.Text strong>{currentPool.name}</Typography.Text>
                            <Button size="small" icon={<EditOutlined />} onClick={() => editCustomPool(currentPool)}>
                              编辑条件
                            </Button>
                          </Space>
                        }
                        bodyStyle={{ padding: '16px' }}
                      >
                        {currentPool.filters.length === 0 ? (
                          <Space direction="vertical" size={12} style={{ display: 'flex', width: '100%' }}>
                            <Empty description="暂无筛选条件" image={Empty.PRESENTED_IMAGE_SIMPLE} />
                            <Button type="primary" size="small" icon={<PlusOutlined />} onClick={() => editCustomPool(currentPool)}>
                              添加筛选条件
                            </Button>
                          </Space>
                        ) : (
                          <Space direction="vertical" size={8} style={{ display: 'flex', width: '100%' }}>
                            {currentPool.filters.map((filter, index) => (
                              <div key={index} className="filter-tag-row">
                                <Tag color="blue" className="filter-tag">{filterFieldOptions.find(f => f.field === filter.field)?.label}</Tag>
                                <Typography.Text className="filter-value">{renderFilterLabel(filter)}</Typography.Text>
                              </div>
                            ))}
                            <Button type="primary" size="small" icon={<PlusOutlined />} onClick={() => editCustomPool(currentPool)}>
                              添加条件
                            </Button>
                          </Space>
                        )}
                      </Card>

                      {/* Stock List */}
                      <Card
                        title={
                          <Space>
                            <SearchOutlined />
                            <span>股票列表</span>
                            <Typography.Text type="secondary">共 {filteredPoolStocks.length} 只</Typography.Text>
                          </Space>
                        }
                        bodyStyle={{ padding: '16px' }}
                      >
                        {allMarketCodes.length === 0 ? (
                          <Skeleton active paragraph={{ rows: 4 }} title={false} />
                        ) : filteredPoolStocks.length === 0 ? (
                          <Empty description="该股票池暂无符合条件的股票" image={Empty.PRESENTED_IMAGE_SIMPLE} />
                        ) : (
                          <>
                            <Table
                              columns={poolStockColumns}
                              dataSource={paginatedPoolStocks}
                              rowKey="code"
                              size="small"
                              pagination={false}
                              scroll={{ x: 'max-content' }}
                            />
                            <Pagination
                              current={poolPage}
                              pageSize={poolPageSize}
                              total={filteredPoolStocks.length}
                              onChange={setPoolPage}
                              style={{ marginTop: 12, textAlign: 'center' }}
                            />
                          </>
                        )}
                      </Card>
                    </Space>
                  ) : (
                    <Card>
                      <Empty description="请选择一个股票池" image={Empty.PRESENTED_IMAGE_SIMPLE} />
                    </Card>
                  )}
                </Col>
              </Row>
            ),
          },
          {
            key: 'allstocks',
            label: (
              <Space>
                <SearchOutlined />
                <span>全部股票</span>
              </Space>
            ),
            children: (
              <Space direction="vertical" size={16} style={{ display: 'flex', width: '100%' }}>
                {/* Filter Panel */}
                <Card
                  title={
                    <Space>
                      <FilterOutlined />
                      <span>筛选条件</span>
                      <Button size="small" type="primary" icon={<SaveOutlined />} onClick={() => setShowSaveAsPoolModal(true)}>
                        保存为股票池
                      </Button>
                      <Button size="small" onClick={resetAllStocksFilters}>重置</Button>
                    </Space>
                  }
                  bodyStyle={{ padding: '16px' }}
                >
                  <Row gutter={[16, 12]}>
                    <Col xs={24} sm={12} lg={8}>
                      <Space direction="vertical" size={4}>
                        <Typography.Text type="secondary">流通市值(亿)</Typography.Text>
                        <Space>
                          <InputNumber
                            min={0}
                            style={{ width: 100 }}
                            placeholder="最小值"
                            value={allStocksFilters.minMarketCap}
                            onChange={(v) => handleAllStocksFilterChange('minMarketCap', v || undefined)}
                          />
                          <span>~</span>
                          <InputNumber
                            min={0}
                            style={{ width: 100 }}
                            placeholder="最大值"
                            value={allStocksFilters.maxMarketCap}
                            onChange={(v) => handleAllStocksFilterChange('maxMarketCap', v || undefined)}
                          />
                        </Space>
                      </Space>
                    </Col>
                    <Col xs={24} sm={12} lg={8}>
                      <Space direction="vertical" size={4}>
                        <Typography.Text type="secondary">股价(元)</Typography.Text>
                        <Space>
                          <InputNumber
                            min={0}
                            style={{ width: 100 }}
                            placeholder="最小值"
                            value={allStocksFilters.minPrice}
                            onChange={(v) => handleAllStocksFilterChange('minPrice', v || undefined)}
                          />
                          <span>~</span>
                          <InputNumber
                            min={0}
                            style={{ width: 100 }}
                            placeholder="最大值"
                            value={allStocksFilters.maxPrice}
                            onChange={(v) => handleAllStocksFilterChange('maxPrice', v || undefined)}
                          />
                        </Space>
                      </Space>
                    </Col>
                    <Col xs={24} sm={12} lg={8}>
                      <Space direction="vertical" size={4}>
                        <Typography.Text type="secondary">换手率(%)</Typography.Text>
                        <Space>
                          <InputNumber
                            min={0}
                            style={{ width: 100 }}
                            placeholder="最小值"
                            value={allStocksFilters.minTurnoverRate}
                            onChange={(v) => handleAllStocksFilterChange('minTurnoverRate', v || undefined)}
                          />
                          <span>~</span>
                          <InputNumber
                            min={0}
                            style={{ width: 100 }}
                            placeholder="最大值"
                            value={allStocksFilters.maxTurnoverRate}
                            onChange={(v) => handleAllStocksFilterChange('maxTurnoverRate', v || undefined)}
                          />
                        </Space>
                      </Space>
                    </Col>
                    <Col xs={24} sm={12} lg={8}>
                      <Space direction="vertical" size={4}>
                        <Typography.Text type="secondary">交易所</Typography.Text>
                        <Select
                          value={allStocksFilters.exchange}
                          onChange={(v) => handleAllStocksFilterChange('exchange', v)}
                          options={[{ value: '', label: '全部' }, ...exchangeOptions]}
                          style={{ width: 150 }}
                          size="small"
                        />
                      </Space>
                    </Col>
                    <Col xs={24} sm={12} lg={8}>
                      <Space direction="vertical" size={4}>
                        <Typography.Text type="secondary">排除ST</Typography.Text>
                        <Switch
                          checked={allStocksFilters.excludeST}
                          onChange={(v) => handleAllStocksFilterChange('excludeST', v)}
                        />
                      </Space>
                    </Col>
                  </Row>
                </Card>

                {/* Stock List */}
                <Card
                  title={
                    <Space>
                      <SearchOutlined />
                      <span>股票列表</span>
                      <Typography.Text type="secondary">共 {allStocksTotal} 只</Typography.Text>
                    </Space>
                  }
                  bodyStyle={{ padding: '16px' }}
                >
                  {allStocksLoading ? (
                    <Skeleton active paragraph={{ rows: 4 }} title={false} />
                  ) : allStocks.length === 0 ? (
                    <Empty description="暂无符合条件的股票" image={Empty.PRESENTED_IMAGE_SIMPLE} />
                  ) : (
                    <>
                      <Table
                        columns={allStockColumns}
                        dataSource={paginatedAllStocks}
                        rowKey="code"
                        size="small"
                        pagination={false}
                        scroll={{ x: 'max-content' }}
                      />
                      <Pagination
                        current={allStocksPage}
                        pageSize={allStocksPageSize}
                        total={allStocksTotal}
                        onChange={setAllStocksPage}
                        style={{ marginTop: 12, textAlign: 'center' }}
                      />
                    </>
                  )}
                </Card>
              </Space>
            ),
          },
        ]}
      />

      {/* Save As Stock Pool Modal */}
      <Modal
        title="保存为股票池"
        open={showSaveAsPoolModal}
        onCancel={() => { setShowSaveAsPoolModal(false); setSaveAsPoolName(''); }}
        footer={[
          <Button key="cancel" onClick={() => { setShowSaveAsPoolModal(false); setSaveAsPoolName(''); }}>取消</Button>,
          <Button key="save" type="primary" onClick={saveAsStockPool} icon={<SaveOutlined />}>保存</Button>,
        ]}
        width={400}
      >
        <Form layout="vertical">
          <Form.Item label="股票池名称" rules={[{ required: true, message: '请输入股票池名称' }]}>
            <Input
              value={saveAsPoolName}
              onChange={(e) => setSaveAsPoolName(e.target.value)}
              placeholder="输入股票池名称"
              autoFocus
            />
          </Form.Item>
        </Form>
        <Alert
          type="info"
          showIcon
          message="提示"
          description="当前筛选条件将作为股票池的条件保存，后续可在自定义股票池中查看和编辑。"
          style={{ marginTop: 12, fontSize: '12px' }}
        />
      </Modal>

      {/* Add/Edit Pool Modal */}
      <Modal
        title={editingPool ? '编辑股票池' : '新建股票池'}
        open={showAddModal || showEditModal}
        onCancel={() => { setShowAddModal(false); setShowEditModal(false); }}
        footer={[
          <Button key="cancel" onClick={() => { setShowAddModal(false); setShowEditModal(false); }}>取消</Button>,
          <Button key="save" type="primary" onClick={savePool} icon={<SaveOutlined />}>保存</Button>,
        ]}
        width={600}
      >
        <Form form={poolForm} layout="vertical">
          <Form.Item label="股票池名称" name="name" rules={[{ required: true, message: '请输入股票池名称' }]}>
            <Input placeholder="输入股票池名称" />
          </Form.Item>
          <Form.Item label="描述" name="description">
            <Input.TextArea placeholder="输入股票池描述（可选）" rows={2} />
          </Form.Item>
        </Form>

        <Divider />

        <Space direction="vertical" size={12} style={{ display: 'flex', width: '100%' }}>
          <Flex justify="space-between" align="center">
            <Typography.Text strong>筛选条件</Typography.Text>
            <Button size="small" type="primary" icon={<PlusOutlined />} onClick={addFilter}>添加条件</Button>
          </Flex>

          {currentFilters.length === 0 ? (
            <Empty description="暂无筛选条件，点击上方按钮添加" image={Empty.PRESENTED_IMAGE_SIMPLE} />
          ) : (
            <List
              dataSource={currentFilters}
              renderItem={(filter, index) => (
                <List.Item
                  extra={<Button size="small" danger onClick={() => removeFilter(index)}>删除</Button>}
                  style={{ padding: '8px 0' }}
                >
                  <Space direction="vertical" size={8}>
                    <Space>
                      <Select
                        value={filter.field}
                        onChange={(v) => updateFilter(index, 'field', v)}
                        options={filterFieldOptions.map(f => ({ value: f.field, label: f.label }))}
                        style={{ width: 140 }}
                        size="small"
                      />
                      <Select
                        value={filter.operator}
                        onChange={(v) => updateFilter(index, 'operator', v)}
                        options={[
                          { value: 'between', label: '区间' },
                          { value: 'gt', label: '大于' },
                          { value: 'gte', label: '大于等于' },
                          { value: 'lt', label: '小于' },
                          { value: 'lte', label: '小于等于' },
                          { value: 'in', label: '包含' },
                        ]}
                        style={{ width: 80 }}
                        size="small"
                      />
                    </Space>
                    {renderFilterField(filter, index)}
                  </Space>
                </List.Item>
              )}
            />
          )}

          <Alert
            type="info"
            showIcon
            message="筛选条件说明"
            description="交易所、板块类型、排除ST条件在前端快速过滤；市值、价格等动态条件暂仅在策略执行时生效。"
            style={{ marginTop: 12, fontSize: '12px' }}
          />
        </Space>
      </Modal>
    </Space>
  );
}

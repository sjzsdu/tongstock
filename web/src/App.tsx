import { Suspense, lazy, useState } from 'react';
import { BrowserRouter, Link, Route, Routes, useLocation, useNavigate } from 'react-router-dom';
import { BarChartOutlined, DashboardOutlined, FundOutlined, HeartOutlined, SearchOutlined, SettingOutlined, StockOutlined } from '@ant-design/icons';
import { Avatar, Breadcrumb, Layout, Menu, Skeleton, Space, Typography } from 'antd';
import type { MenuProps } from 'antd';
import StockSearchInput from './components/StockSearchInput';

const Dashboard = lazy(() => import('./pages/Dashboard'));
const StockDetail = lazy(() => import('./pages/stock/StockDetail'));
const StockChoose = lazy(() => import('./pages/stock/StockChoose'));
const Screen = lazy(() => import('./pages/Screen'));
const Blocks = lazy(() => import('./pages/Blocks'));
const Watchlist = lazy(() => import('./pages/Watchlist'));
const SettingsPage = lazy(() => import('./pages/settings/SettingsPage'));

const { Header, Content, Sider } = Layout;

function GlobalSearch() {
  const navigate = useNavigate();

  return (
    <div style={{ width: 320, maxWidth: '100%' }}>
      <StockSearchInput
        placeholder="输入代码、名称或拼音..."
        limit={8}
        containerClassName="global-stock-search"
        onSelect={(match) => navigate(`/stock/${match.code}`)}
      />
    </div>
  );
}

function RouteFallback() {
  return (
    <Space direction="vertical" size={16} style={{ display: 'flex', width: '100%' }}>
      <Skeleton.Button active block style={{ width: 240, height: 40 }} />
      <Skeleton active paragraph={{ rows: 4 }} />
      <Skeleton active paragraph={{ rows: 8 }} />
    </Space>
  );
}

function AppLayout({ children }: { children: React.ReactNode }) {
  const location = useLocation();
  const [collapsed, setCollapsed] = useState(false);
  const selectedKey = location.pathname.startsWith('/stock')
    ? '/stock/choose'
    : location.pathname.startsWith('/screen')
      ? '/screen'
      : location.pathname.startsWith('/blocks')
        ? '/blocks'
        : location.pathname.startsWith('/watchlist')
          ? '/watchlist'
          : location.pathname.startsWith('/settings')
            ? '/settings'
            : '/';

  const menuItems: MenuProps['items'] = [
    { key: '/', icon: <DashboardOutlined />, label: <Link to="/">市场总览</Link> },
    { key: '/watchlist', icon: <HeartOutlined />, label: <Link to="/watchlist">自选股</Link> },
    { key: '/stock/choose', icon: <BarChartOutlined />, label: <Link to="/stock/choose">个股分析</Link> },
    { key: '/screen', icon: <SearchOutlined />, label: <Link to="/screen">信号筛选</Link> },
    { key: '/blocks', icon: <FundOutlined />, label: <Link to="/blocks">板块热点</Link> },
    { key: '/settings', icon: <SettingOutlined />, label: <Link to="/settings">配置</Link> },
  ];

  const breadcrumbItems = buildBreadcrumbs(location.pathname);

  return (
    <Layout>
      <Sider
        width={240}
        collapsedWidth={72}
        collapsible
        collapsed={collapsed}
        onCollapse={setCollapsed}
        theme="dark"
        style={{ borderRight: '1px solid #1f2937' }}
      >
        <div style={{ padding: collapsed ? '20px 12px' : 20, borderBottom: '1px solid #1f2937' }}>
          <Space align="center" size={12}>
            <Avatar shape="square" icon={<StockOutlined />} style={{ backgroundColor: '#1677ff' }} />
            {!collapsed && (
              <div>
                <Typography.Title level={4} style={{ margin: 0, color: '#fff' }}>
                  TongStock
                </Typography.Title>
                <Typography.Text type="secondary">A 股分析工作台</Typography.Text>
              </div>
            )}
          </Space>
        </div>
        <Menu
          mode="inline"
          theme="dark"
          selectedKeys={[selectedKey]}
          items={menuItems}
          style={{ borderInlineEnd: 0, paddingTop: 12 }}
        />
      </Sider>
      <Layout>
        <Header
          style={{
            padding: '0 24px',
            display: 'flex',
            alignItems: 'center',
            justifyContent: 'space-between',
            borderBottom: '1px solid #1f2937',
          }}
        >
          <Breadcrumb items={breadcrumbItems} />
          <GlobalSearch />
        </Header>
        <Content style={{ padding: 24, overflow: 'auto' }}>
          <Suspense fallback={<RouteFallback />}>
            {children}
          </Suspense>
        </Content>
      </Layout>
    </Layout>
  );
}

export default function App() {
  return (
    <BrowserRouter basename={import.meta.env.BASE_URL}>
      <AppLayout>
        <Routes>
          <Route path="/" element={<Dashboard />} />
          <Route path="/watchlist" element={<Watchlist />} />
          <Route path="/stock/choose" element={<StockChoose />} />
          <Route path="/stock/:code" element={<StockDetail />} />
          <Route path="/stock/:code/:tab" element={<StockDetail />} />
          <Route path="/screen" element={<Screen />} />
          <Route path="/blocks" element={<Blocks />} />
          <Route path="/settings" element={<SettingsPage />} />
        </Routes>
      </AppLayout>
    </BrowserRouter>
  );
}

function buildBreadcrumbs(pathname: string) {
  const parts = pathname.split('/').filter(Boolean);
  const items: { title: React.ReactNode }[] = [{ title: <Link to="/">TongStock</Link> }];

  if (parts.length === 0) {
    items.push({ title: '市场总览' });
    return items;
  }

  const labels: Record<string, string> = {
    stock: '个股分析',
    choose: '选择股票',
    watchlist: '自选股',
    screen: '信号筛选',
    blocks: '板块热点',
    settings: '配置',
    chart: 'K线+指标',
    signal: '信号',
    finance: '财务',
    company: '公司',
    dividend: '分红',
    intraday: '分时',
  };

  let current = '';
  parts.forEach((part, index) => {
    current += `/${part}`;
    items.push({
      title: index === parts.length - 1 ? (labels[part] ?? part) : <Link to={current}>{labels[part] ?? part}</Link>,
    });
  });

  return items;
}

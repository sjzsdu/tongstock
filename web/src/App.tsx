import { Suspense, lazy, useEffect, useState } from 'react';
import { BrowserRouter, Link, Route, Routes, useLocation, useNavigate } from 'react-router-dom';
import { BarChartOutlined, DashboardOutlined, FileTextOutlined, FundOutlined, HeartOutlined, MenuOutlined, RadarChartOutlined, RobotOutlined, SearchOutlined, SettingOutlined, StockOutlined, WalletOutlined } from '@ant-design/icons';
import { Avatar, Breadcrumb, Button, Drawer, Layout, Menu, Skeleton, Space, Typography } from 'antd';
import type { MenuProps } from 'antd';
import StockSearchInput from './components/StockSearchInput';

const Dashboard = lazy(() => import('./pages/Dashboard'));
const StockDetail = lazy(() => import('./pages/stock/StockDetail'));
const StockChoose = lazy(() => import('./pages/stock/StockChoose'));
const Screen = lazy(() => import('./pages/Screen'));
const Blocks = lazy(() => import('./pages/Blocks'));
const Watchlist = lazy(() => import('./pages/Watchlist'));
const Portfolio = lazy(() => import('./pages/Portfolio'));
const SettingsPage = lazy(() => import('./pages/settings/SettingsPage'));
const IndexDetail = lazy(() => import('./pages/index/IndexDetail'));
const AgentWeb = lazy(() => import('./pages/AgentWeb'));
const Paradigms = lazy(() => import('./pages/Paradigms'));
const OvernightArbitrage = lazy(() => import('./pages/strategy/OvernightArbitrage'));
const EventDetail = lazy(() => import('./pages/news/EventDetail'));
const NewsHome = lazy(() => import('./pages/news/NewsHome'));

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
  const [drawerOpen, setDrawerOpen] = useState(false);
  const [isMobile, setIsMobile] = useState(false);

  useEffect(() => {
    const checkMobile = () => {
      setIsMobile(window.innerWidth < 768);
      if (window.innerWidth < 768) {
        setCollapsed(true);
      }
    };
    checkMobile();
    window.addEventListener('resize', checkMobile);
    return () => window.removeEventListener('resize', checkMobile);
  }, []);

  const selectedKey = location.pathname.startsWith('/stock')
    ? '/stock/choose'
    : location.pathname.startsWith('/index')
      ? '/'
      : location.pathname.startsWith('/screen')
        ? '/screen'
        : location.pathname.startsWith('/portfolio')
          ? '/portfolio'
          : location.pathname.startsWith('/blocks')
            ? '/blocks'
            : location.pathname.startsWith('/watchlist')
              ? '/watchlist'
              : location.pathname.startsWith('/settings')
                ? '/settings'
                : location.pathname.startsWith('/agent')
                  ? '/agent'
                  : location.pathname.startsWith('/paradigms')
                    ? '/paradigms'
                    : location.pathname.startsWith('/strategy')
                      ? '/strategy/overnight'
                      : location.pathname.startsWith('/news')
                        ? '/news'
                        : '/';

  const menuItems: MenuProps['items'] = [
    { key: '/', icon: <DashboardOutlined />, label: <Link to="/">市场总览</Link> },
    { key: '/news', icon: <FileTextOutlined />, label: <Link to="/news">财经资讯</Link> },
    { key: '/watchlist', icon: <HeartOutlined />, label: <Link to="/watchlist">自选股</Link> },
    { key: '/stock/choose', icon: <BarChartOutlined />, label: <Link to="/stock/choose">个股分析</Link> },
    { key: '/screen', icon: <SearchOutlined />, label: <Link to="/screen">信号筛选</Link> },
    { key: '/portfolio', icon: <WalletOutlined />, label: <Link to="/portfolio">虚拟持仓</Link> },
    { key: '/blocks', icon: <FundOutlined />, label: <Link to="/blocks">股票池管理</Link> },
    { key: '/settings', icon: <SettingOutlined />, label: <Link to="/settings">配置</Link> },
    { key: '/agent', icon: <RobotOutlined />, label: <Link to="/agent">AI 助手</Link> },
    { key: '/paradigms', icon: <RadarChartOutlined />, label: <Link to="/paradigms">范式管理</Link> },
    { key: '/strategy/overnight', icon: <SearchOutlined />, label: <Link to="/strategy/overnight">隔夜套利</Link> },
  ];

  const breadcrumbItems = buildBreadcrumbs(location.pathname);

  return (
    <Layout>
      {!isMobile && (
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
      )}
      <Layout>
        <Header
          style={{
            padding: isMobile ? '8px 12px' : '0 24px',
            display: 'flex',
            alignItems: 'center',
            justifyContent: isMobile ? 'space-between' : 'space-between',
            borderBottom: '1px solid #1f2937',
            flexWrap: isMobile ? 'wrap' : 'nowrap',
            rowGap: isMobile ? '8px' : 0,
          }}
        >
          <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
            {isMobile && (
              <Button type="text" icon={<MenuOutlined />} onClick={() => setDrawerOpen(true)} style={{ color: '#fff' }} />
            )}
            <Breadcrumb items={breadcrumbItems} style={{ flexShrink: 0 }} />
          </div>
          <GlobalSearch />
        </Header>
        <Content style={{ padding: isMobile ? 12 : 24, overflow: 'auto' }}>
          <Suspense fallback={<RouteFallback />}>
            {children}
          </Suspense>
        </Content>
      </Layout>

      {/* Mobile Drawer */}
      <Drawer
        title="TongStock"
        placement="left"
        open={drawerOpen}
        onClose={() => setDrawerOpen(false)}
        width={240}
        style={{ background: '#0b1220' }}
        headerStyle={{ background: '#1f2937', color: '#fff' }}
      >
        <Menu
          mode="inline"
          theme="dark"
          selectedKeys={[selectedKey]}
          items={menuItems}
          onClick={() => setDrawerOpen(false)}
          style={{ borderInlineEnd: 0, paddingTop: 12 }}
        />
      </Drawer>
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
          <Route path="/portfolio" element={<Portfolio />} />
          <Route path="/blocks" element={<Blocks />} />
          <Route path="/settings" element={<SettingsPage />} />
          <Route path="/index/:code" element={<IndexDetail />} />
          <Route path="/index/:code/:tab" element={<IndexDetail />} />
          <Route path="/agent" element={<AgentWeb />} />
          <Route path="/paradigms" element={<Paradigms />} />
          <Route path="/strategy/overnight" element={<OvernightArbitrage />} />
          <Route path="/news" element={<NewsHome />} />
          <Route path="/news/event/:id" element={<EventDetail />} />
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
    portfolio: '虚拟持仓',
    blocks: '股票池管理',
    settings: '配置',
    paradigms: '范式管理',
    strategy: '策略',
    overnight: '隔夜套利',
    chart: 'K线+指标',
    signal: '信号',
    finance: '财务',
    company: '公司',
    dividend: '分红',
    intraday: '分时',
    index: '指数详情',
    stats: '涨跌统计',
    components: '成分股',
    news: '财经资讯',
    event: '热点事件',
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

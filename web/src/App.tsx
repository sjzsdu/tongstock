import { Suspense, lazy, useEffect, useState } from 'react';
import type { ComponentType } from 'react';
import { BrowserRouter, Link, Route, Routes, useLocation, useNavigate } from 'react-router-dom';
import {
  CloseOutlined,
  DashboardOutlined,
  FileTextOutlined,
  HeartOutlined,
  MenuOutlined,
  RadarChartOutlined,
  RobotOutlined,
  SearchOutlined,
  SettingOutlined,
  StockOutlined,
  SafetyCertificateOutlined,
  WalletOutlined,
  BlockOutlined,
  FormOutlined,
} from '@ant-design/icons';
import { Avatar, Breadcrumb, Button, Drawer, Layout, Menu, Skeleton, Space, Typography } from 'antd';
import type { MenuProps } from 'antd';
import StockSearchInput from './components/StockSearchInput';
import ErrorBoundary from './components/ErrorBoundary';
import { useDocumentTitle } from './hooks/useDocumentTitle';
import { titleForPath } from './lib/pageTitle';

const Dashboard = lazy(lazyWithReload(() => import('./pages/Dashboard')));
const StockDetail = lazy(lazyWithReload(() => import('./pages/stock/StockDetail')));
const StockChoose = lazy(lazyWithReload(() => import('./pages/stock/StockChoose')));
const Screen = lazy(lazyWithReload(() => import('./pages/Screen')));
const Blocks = lazy(lazyWithReload(() => import('./pages/Blocks')));
const Watchlist = lazy(lazyWithReload(() => import('./pages/Watchlist')));
const Portfolio = lazy(lazyWithReload(() => import('./pages/Portfolio')));
const SettingsPage = lazy(lazyWithReload(() => import('./pages/settings/SettingsPage')));
const IndexDetail = lazy(lazyWithReload(() => import('./pages/index/IndexDetail')));
const AgentWeb = lazy(lazyWithReload(() => import('./pages/AgentWeb')));
const Paradigms = lazy(lazyWithReload(() => import('./pages/Paradigms')));
const OvernightArbitrage = lazy(lazyWithReload(() => import('./pages/strategy/OvernightArbitrage')));
const EventDetail = lazy(lazyWithReload(() => import('./pages/news/EventDetail')));
const NewsHome = lazy(lazyWithReload(() => import('./pages/news/NewsHome')));
const Monitoring = lazy(lazyWithReload(() => import('./pages/Monitoring')));
const Methods = lazy(lazyWithReload(() => import('./pages/Methods')));
const SelectionResult = lazy(lazyWithReload(() => import('./pages/SelectionResult')));
const NotFound = lazy(lazyWithReload(() => import('./pages/NotFound')));

// 懒加载 chunk 失败（重新部署后旧标签页请求旧 hash 文件返回 404）时，
// 自动整页刷新一次拿新版本。sessionStorage 标记防止刷新后仍失败时死循环。
function lazyWithReload<T extends { default: ComponentType<any> }>(factory: () => Promise<T>) {
  return () =>
    factory().catch((error: unknown) => {
      const key = 'tongstock:chunk-reloaded';
      if (!sessionStorage.getItem(key)) {
        sessionStorage.setItem(key, '1');
        window.location.reload();
      }
      throw error;
    });
}

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

// 决策组：产品核心主流程，顶层平铺、零点击可见
const decisionMenuItems: MenuProps['items'] = [
  { key: '/', icon: <DashboardOutlined />, label: <Link to="/">今日决策</Link> },
  { key: '/portfolio', icon: <WalletOutlined />, label: <Link to="/portfolio">持仓卖出</Link> },
];

// 方法研究组：可信方法、范式体系与 AI 研究入口
const researchMenuItems: MenuProps['items'] = [
  { key: '/methods', icon: <SafetyCertificateOutlined />, label: <Link to="/methods">可信方法</Link> },
  { key: '/paradigms', icon: <RadarChartOutlined />, label: <Link to="/paradigms">范式库</Link> },
  { key: '/monitoring', icon: <SafetyCertificateOutlined />, label: <Link to="/monitoring">范式监控</Link> },
  { key: '/agent', icon: <RobotOutlined />, label: <Link to="/agent">AI 助手</Link> },
];

// 行情工具组：个股浏览与行情分析工具
const marketMenuItems: MenuProps['items'] = [
  { key: '/watchlist', icon: <HeartOutlined />, label: <Link to="/watchlist">自选股</Link> },
  { key: '/stock/choose', icon: <StockOutlined />, label: <Link to="/stock/choose">个股分析</Link> },
  { key: '/blocks', icon: <BlockOutlined />, label: <Link to="/blocks">股票池</Link> },
  { key: '/screen', icon: <SearchOutlined />, label: <Link to="/screen">信号筛选</Link> },
  { key: '/strategy/overnight', icon: <FormOutlined />, label: <Link to="/strategy/overnight">隔夜套利</Link> },
  { key: '/news', icon: <FileTextOutlined />, label: <Link to="/news">财经资讯</Link> },
];

// 系统组：设置与扩展点
const systemMenuItems: MenuProps['items'] = [
  { key: '/settings', icon: <SettingOutlined />, label: <Link to="/settings">配置</Link> },
];

const menuItems: MenuProps['items'] = [
  ...decisionMenuItems,
  { key: 'research-submenu', icon: <RadarChartOutlined />, label: '方法研究', children: researchMenuItems },
  { key: 'market-submenu', icon: <StockOutlined />, label: '行情工具', children: marketMenuItems },
  { key: 'system-submenu', icon: <SettingOutlined />, label: '系统', children: systemMenuItems },
];

// 子菜单 key -> 组内成员路由，用于路由变化时自动展开所属分组
const SUBMENU_BY_KEY: Record<string, string> = {
  '/methods': 'research-submenu',
  '/paradigms': 'research-submenu',
  '/monitoring': 'research-submenu',
  '/agent': 'research-submenu',
  '/watchlist': 'market-submenu',
  '/stock/choose': 'market-submenu',
  '/blocks': 'market-submenu',
  '/screen': 'market-submenu',
  '/strategy/overnight': 'market-submenu',
  '/news': 'market-submenu',
  '/settings': 'system-submenu',
};

const DEFAULT_OPEN_KEYS = ['research-submenu', 'market-submenu'];

function getParentKey(pathname: string): string | undefined {
  return SUBMENU_BY_KEY[getSelectedKey(pathname)];
}

// 计算选中的菜单 key
function getSelectedKey(pathname: string): string {
  if (pathname === '/') return '/';
  if (pathname.startsWith('/stock')) return '/stock/choose';
  if (pathname.startsWith('/index')) return '/stock/choose';
  if (pathname.startsWith('/screen')) return '/screen';
  if (pathname.startsWith('/portfolio')) return '/portfolio';
  if (pathname.startsWith('/methods')) return '/methods';
  if (pathname.startsWith('/blocks')) return '/blocks';
  if (pathname.startsWith('/watchlist')) return '/watchlist';
  if (pathname.startsWith('/settings')) return '/settings';
  if (pathname.startsWith('/agent')) return '/agent';
  if (pathname.startsWith('/paradigms')) return '/paradigms';
  if (pathname.startsWith('/strategy')) return '/strategy/overnight';
  if (pathname.startsWith('/news')) return '/news';
  return '/';
}

function AppLayout({ children }: { children: React.ReactNode }) {
  const location = useLocation();
  const [collapsed, setCollapsed] = useState(false);
  const [drawerOpen, setDrawerOpen] = useState(false);
  const [isMobile, setIsMobile] = useState(false);
  // 极窄视口（<360px）：顶部放不下常驻搜索框，折叠成图标按需展开
  const [isUltraNarrow, setIsUltraNarrow] = useState(false);
  const [searchExpanded, setSearchExpanded] = useState(false);
  const [openKeys, setOpenKeys] = useState<string[]>(DEFAULT_OPEN_KEYS);
  const navigate = useNavigate();

  // 浏览器标签页标题：路由级标题，详情页（个股/事件）拿到数据后会覆盖
  useDocumentTitle(titleForPath(location.pathname));

  // 路由变化时自动展开选中项所属分组
  useEffect(() => {
    const parent = getParentKey(location.pathname);
    if (parent) {
      setOpenKeys((prev) => (prev.includes(parent) ? prev : [...prev, parent]));
    }
  }, [location.pathname]);

  useEffect(() => {
    const checkMobile = () => {
      setIsMobile(window.innerWidth < 768);
      setIsUltraNarrow(window.innerWidth < 360);
      if (window.innerWidth < 768) {
        setCollapsed(true);
      }
    };
    checkMobile();
    window.addEventListener('resize', checkMobile);
    return () => window.removeEventListener('resize', checkMobile);
  }, []);

  const selectedKey = getSelectedKey(location.pathname);
  const breadcrumbItems = buildBreadcrumbs(location.pathname);
  // AI 助手是对话式整屏布局：去掉内容区内边距，让它自己铺满并内部滚动
  const isFullscreen = location.pathname.startsWith('/agent');

  return (
    <Layout className="app-shell">
      {!isMobile && (
        <Sider
          width={220}
          collapsedWidth={72}
          collapsible
          collapsed={collapsed}
          onCollapse={setCollapsed}
          theme="dark"
          style={{ borderRight: '1px solid #1f2937' }}
        >
          <div
            style={{
              padding: collapsed ? '20px 12px' : 16,
              borderBottom: '1px solid #1f2937',
              flex: '0 0 auto',
            }}
          >
            <Space align="center" size={12}>
              <Avatar shape="square" icon={<StockOutlined />} style={{ backgroundColor: '#1677ff' }} />
              {!collapsed && (
                <div>
                  <Typography.Title level={4} style={{ margin: 0, color: '#fff', fontSize: 16 }}>
                    TongStock
                  </Typography.Title>
                  <Typography.Text type="secondary" style={{ fontSize: 11 }}>
                    AI 投资决策
                  </Typography.Text>
                </div>
              )}
            </Space>
          </div>
          <div className="app-menu-scroll">
            <Menu
              mode="inline"
              theme="dark"
              selectedKeys={[selectedKey]}
              openKeys={openKeys}
              onOpenChange={setOpenKeys}
              items={menuItems}
              style={{ borderInlineEnd: 0, paddingTop: 8 }}
            />
          </div>
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
            flex: '0 0 auto',
          }}
        >
          <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
            {isMobile && (
              <Button type="text" icon={<MenuOutlined />} onClick={() => setDrawerOpen(true)} style={{ color: '#fff' }} />
            )}
            <Breadcrumb items={breadcrumbItems} style={{ flexShrink: 0 }} />
          </div>
          {isUltraNarrow ? (
            <Button
              type="text"
              icon={searchExpanded ? <CloseOutlined /> : <SearchOutlined />}
              onClick={() => setSearchExpanded((v) => !v)}
              style={{ color: '#fff' }}
              aria-label={searchExpanded ? '收起搜索' : '展开搜索'}
            />
          ) : (
            <GlobalSearch />
          )}
          {/* 极窄屏展开后的搜索框：独占第二行，选中后自动收起 */}
          {isUltraNarrow && searchExpanded && (
            <div style={{ flex: '1 1 100%', minWidth: 0 }}>
              <StockSearchInput
                placeholder="输入代码、名称或拼音..."
                limit={8}
                containerClassName="global-stock-search"
                autoFocus
                onSelect={(match) => {
                  setSearchExpanded(false);
                  navigate(`/stock/${match.code}`);
                }}
              />
            </div>
          )}
        </Header>
        <Content
          className={`app-content${isFullscreen ? ' app-content-fill' : ''}`}
          style={{ padding: isFullscreen ? 0 : isMobile ? 12 : 24 }}
        >
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
        <ErrorBoundary>
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
            <Route path="/monitoring" element={<Monitoring />} />
            <Route path="/methods" element={<Methods />} />
            <Route path="/methods/selection/:runId" element={<SelectionResult />} />
            <Route path="/strategy/overnight" element={<OvernightArbitrage />} />
            <Route path="/news" element={<NewsHome />} />
            <Route path="/news/event/:id" element={<EventDetail />} />
            <Route path="*" element={<NotFound />} />
          </Routes>
        </ErrorBoundary>
      </AppLayout>
    </BrowserRouter>
  );
}

function buildBreadcrumbs(pathname: string) {
  const parts = pathname.split('/').filter(Boolean);
  const items: { title: React.ReactNode }[] = [{ title: <Link to="/">TongStock</Link> }];

  if (parts.length === 0) {
    items.push({ title: '首页' });
    return items;
  }

  const labels: Record<string, string> = {
    stock: '个股分析',
    choose: '选择股票',
    watchlist: '自选股',
    screen: '信号筛选',
    portfolio: '持仓卖出',
    blocks: '股票池',
    settings: '配置',
    paradigms: '范式库',
    monitoring: '范式监控',
    methods: '可信方法库',
    selection: '选股结果',
    strategy: '策略',
    overnight: '隔夜套利',
    agent: 'AI 助手',
    index: '指数详情',
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

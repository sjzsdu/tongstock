import { BrowserRouter, Routes, Route, Link, useLocation } from 'react-router-dom';
import { TrendingUp, LayoutDashboard, Search } from 'lucide-react';
import Dashboard from './pages/Dashboard';
import Stock from './pages/Stock';
import Screen from './pages/Screen';

function NavLink({ to, children, icon: Icon }: { to: string; children: React.ReactNode; icon: any }) {
  const location = useLocation();
  const active = location.pathname === to || (to !== '/' && location.pathname.startsWith(to));
  return (
    <Link
      to={to}
      className={`flex items-center gap-2 px-4 py-2 rounded-lg transition-colors ${
        active ? 'bg-blue-600 text-white' : 'text-slate-400 hover:text-white hover:bg-slate-800'
      }`}
    >
      <Icon size={18} />
      {children}
    </Link>
  );
}

function Layout({ children }: { children: React.ReactNode }) {
  return (
    <div className="flex min-h-screen">
      <nav className="w-56 bg-slate-900 border-r border-slate-800 p-4 flex flex-col gap-1">
        <div className="flex items-center gap-2 px-2 mb-6">
          <TrendingUp className="text-blue-500" size={24} />
          <span className="text-lg font-bold text-white">TongStock</span>
        </div>
        <NavLink to="/" icon={LayoutDashboard}>市场总览</NavLink>
        <NavLink to="/stock" icon={TrendingUp}>指标分析</NavLink>
        <NavLink to="/screen" icon={Search}>信号筛选</NavLink>
      </nav>
      <main className="flex-1 p-6 overflow-auto">
        {children}
      </main>
    </div>
  );
}

export default function App() {
  return (
    <BrowserRouter>
      <Layout>
        <Routes>
          <Route path="/" element={<Dashboard />} />
          <Route path="/stock" element={<Stock />} />
          <Route path="/stock/:code" element={<Stock />} />
          <Route path="/screen" element={<Screen />} />
        </Routes>
      </Layout>
    </BrowserRouter>
  );
}

import { BrowserRouter, Routes, Route, Link, useLocation, useNavigate } from 'react-router-dom';
import { TrendingUp, LayoutDashboard, Search, Settings, BarChart3 } from 'lucide-react';
import { useState, useEffect, useRef } from 'react';
import Dashboard from './pages/Dashboard';
import StockDetail from './pages/stock/StockDetail';
import StockChoose from './pages/stock/StockChoose';
import Screen from './pages/Screen';
import SettingsPage from './pages/settings/SettingsPage';
import { api } from './api/client';
import type { CodeItem } from './types/api';

function SearchBar() {
  const [query, setQuery] = useState('');
  const [codes, setCodes] = useState<CodeItem[]>([]);
  const [results, setResults] = useState<CodeItem[]>([]);
  const [open, setOpen] = useState(false);
  const ref = useRef<HTMLDivElement>(null);
  const navigate = useNavigate();

  useEffect(() => {
    Promise.all([
      api.codes('sz').catch(() => []),
      api.codes('sh').catch(() => []),
    ]).then(([sz, sh]) => setCodes([...sz, ...sh]));
  }, []);

  useEffect(() => {
    if (query.length < 1) { setResults([]); return; }
    const q = query.toLowerCase();
    setResults(codes.filter(c =>
      c.Code.includes(q) || c.Name.toLowerCase().includes(q)
    ).slice(0, 10));
  }, [query, codes]);

  useEffect(() => {
    const handler = (e: MouseEvent) => {
      if (ref.current && !ref.current.contains(e.target as Node)) setOpen(false);
    };
    document.addEventListener('click', handler);
    return () => document.removeEventListener('click', handler);
  }, []);

  const go = (code: string) => {
    setQuery('');
    setOpen(false);
    navigate(`/stock/${code}`);
  };

  return (
    <div ref={ref} className="relative">
      <div className="flex items-center bg-slate-800 rounded-lg border border-slate-700 focus-within:border-blue-500">
        <Search size={16} className="ml-3 text-slate-500" />
        <input
          type="text"
          value={query}
          onChange={e => { setQuery(e.target.value); setOpen(true); }}
          onFocus={() => setOpen(true)}
          onKeyDown={e => { if (e.key === 'Enter' && results.length > 0) go(results[0].Code); }}
          placeholder="输入代码或名称搜索..."
          className="bg-transparent px-3 py-2 text-white text-sm w-64 focus:outline-none"
        />
      </div>
      {open && results.length > 0 && (
        <div className="absolute top-full mt-1 w-full bg-slate-800 border border-slate-700 rounded-lg shadow-xl z-50 max-h-64 overflow-auto">
          {results.map(c => (
            <button
              key={c.Code}
              onClick={() => go(c.Code)}
              className="w-full text-left px-4 py-2 hover:bg-slate-700 flex justify-between text-sm"
            >
              <span className="text-blue-400 font-mono">{c.Code}</span>
              <span className="text-slate-300">{c.Name}</span>
            </button>
          ))}
        </div>
      )}
    </div>
  );
}

function NavLink({ to, children, icon: Icon }: { to: string; children: React.ReactNode; icon: any }) {
  const loc = useLocation();
  const active = loc.pathname === to || (to !== '/' && loc.pathname.startsWith(to));
  return (
    <Link to={to} className={`flex items-center gap-2 px-4 py-2 rounded-lg transition-colors ${
      active ? 'bg-blue-600 text-white' : 'text-slate-400 hover:text-white hover:bg-slate-800'
    }`}>
      <Icon size={18} /> {children}
    </Link>
  );
}

function Layout({ children }: { children: React.ReactNode }) {
  return (
    <div className="flex h-screen overflow-hidden">
      <nav className="w-56 bg-slate-900 border-r border-slate-800 p-4 flex flex-col gap-1 shrink-0">
        <Link to="/" className="flex items-center gap-2 px-2 mb-6">
          <TrendingUp className="text-blue-500" size={24} />
          <span className="text-lg font-bold text-white">TongStock</span>
        </Link>
        <NavLink to="/" icon={LayoutDashboard}>市场总览</NavLink>
        <NavLink to="/stock/choose" icon={BarChart3}>个股分析</NavLink>
        <NavLink to="/screen" icon={Search}>信号筛选</NavLink>
        <NavLink to="/settings" icon={Settings}>配置</NavLink>
        <div className="mt-auto pt-4 border-t border-slate-800">
          <SearchBar />
        </div>
      </nav>
      <main className="flex-1 p-6 overflow-auto bg-slate-950">{children}</main>
    </div>
  );
}

export default function App() {
  return (
    <BrowserRouter>
      <Layout>
        <Routes>
          <Route path="/" element={<Dashboard />} />
          <Route path="/stock/choose" element={<StockChoose />} />
          <Route path="/stock/:code" element={<StockDetail />} />
          <Route path="/stock/:code/:tab" element={<StockDetail />} />
          <Route path="/screen" element={<Screen />} />
          <Route path="/settings" element={<SettingsPage />} />
        </Routes>
      </Layout>
    </BrowserRouter>
  );
}

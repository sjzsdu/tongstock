import { useState, useEffect } from 'react';
import { useNavigate } from 'react-router-dom';
import { Search } from 'lucide-react';
import { api } from '../../api/client';
import type { CodeItem } from '../../types/api';

export default function StockChoose() {
  const navigate = useNavigate();
  const [inputCode, setInputCode] = useState('');
  const [codes, setCodes] = useState<CodeItem[]>([]);
  const [results, setResults] = useState<CodeItem[]>([]);
  const [showResults, setShowResults] = useState(false);

  // 加载股票列表
  useEffect(() => {
    Promise.all([
      api.codes('sz').catch(() => []),
      api.codes('sh').catch(() => []),
    ]).then(([sz, sh]) => setCodes([...sz, ...sh]));
  }, []);

  const handleSearch = (query: string) => {
    setInputCode(query);
    if (query.length < 1) {
      setResults([]);
      setShowResults(false);
      return;
    }
    const q = query.toLowerCase();
    setResults(codes.filter(c =>
      c.Code.includes(q) || c.Name.toLowerCase().includes(q)
    ).slice(0, 20));
    setShowResults(true);
  };

  const selectStock = (code: string) => {
    navigate(`/stock/${code}`);
  };

  const handleKeyDown = (e: React.KeyboardEvent) => {
    if (e.key === 'Enter' && inputCode.length === 6) {
      selectStock(inputCode);
    }
  };

  return (
    <div className="flex flex-col items-center justify-center h-full min-h-0">
      <div className="w-full max-w-lg">
        <h1 className="text-2xl font-bold text-white mb-8 text-center">个股分析</h1>
        
        <div className="relative">
          <div className="flex items-center bg-slate-800 rounded-lg border border-slate-700 focus-within:border-blue-500">
            <Search size={20} className="ml-4 text-slate-500" />
            <input
              type="text"
              value={inputCode}
              onChange={e => handleSearch(e.target.value)}
              onKeyDown={handleKeyDown}
              onFocus={() => setShowResults(true)}
              placeholder="输入股票代码或名称搜索..."
              className="flex-1 bg-transparent px-4 py-4 text-white text-lg focus:outline-none"
            />
          </div>
          
          {showResults && results.length > 0 && (
            <div className="absolute top-full mt-2 w-full bg-slate-800 border border-slate-700 rounded-lg shadow-xl z-50 max-h-80 overflow-auto">
              {results.map(c => (
                <button
                  key={c.Code}
                  onClick={() => selectStock(c.Code)}
                  className="w-full text-left px-4 py-3 hover:bg-slate-700 flex justify-between items-center border-b border-slate-700/50 last:border-0"
                >
                  <span className="text-blue-400 font-mono text-lg">{c.Code}</span>
                  <span className="text-slate-300 text-lg">{c.Name}</span>
                </button>
              ))}
            </div>
          )}
          
          {showResults && inputCode.length >= 6 && results.length === 0 && (
            <div className="absolute top-full mt-2 w-full bg-slate-800 border border-slate-700 rounded-lg shadow-xl z-50 p-4 text-center text-slate-400">
              未找到股票 "{inputCode}"
            </div>
          )}
        </div>

        <div className="mt-8 text-center text-slate-500 text-sm">
          <p>输入6位股票代码，按回车进入详情页</p>
          <p>或输入股票名称/代码进行搜索</p>
        </div>
      </div>
    </div>
  );
}
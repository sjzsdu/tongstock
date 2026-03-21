import { useState } from 'react';
import { Settings, Save } from 'lucide-react';

const DEFAULT_CONFIG = `# TongStock 指标参数配置

defaults:
  ma: [5, 10, 20, 60]
  macd:
    fast: 12
    slow: 26
    signal: 9
  kdj:
    n: 9
    m1: 3
    m2: 3
  boll:
    n: 20
    2.0
  rsi: [6, 14]

categories:
  large_cap:
    ma: [5, 10, 20, 60, 120]
    macd:
      fast: 12
      slow: 26
      signal: 9
  small_cap:
    ma: [5, 10, 20]
    macd:
      fast: 8
      slow: 17
      signal: 9
    kdj:
      n: 7
      m1: 3
      m2: 3

overrides:
  "000001":
    kdj:
      n: 5
      m1: 3
      m2: 3`;

export default function SettingsPage() {
  const [config, setConfig] = useState(DEFAULT_CONFIG);
  const [saved, setSaved] = useState(false);

  const handleSave = () => {
    setSaved(true);
    setTimeout(() => setSaved(false), 2000);
  };

  return (
    <div className="space-y-6 max-w-4xl">
      <h1 className="text-2xl font-bold text-white flex items-center gap-2">
        <Settings size={24} /> 配置
      </h1>

      <div className="bg-slate-900 rounded-xl border border-slate-800 p-6">
        <h2 className="text-white font-medium mb-4">指标参数配置 (indicator.yaml)</h2>
        <p className="text-slate-400 text-sm mb-4">
          配置文件路径: ~/.tongstock/indicator.yaml
        </p>
        <textarea
          value={config}
          onChange={e => setConfig(e.target.value)}
          className="w-full h-96 bg-slate-800 border border-slate-700 rounded-lg p-4 text-sm text-slate-200 font-mono focus:outline-none focus:border-blue-500 resize-y"
          spellCheck={false}
        />
        <div className="flex items-center gap-4 mt-4">
          <button
            onClick={handleSave}
            className="flex items-center gap-2 bg-blue-600 hover:bg-blue-700 px-4 py-2 rounded-lg text-white font-medium"
          >
            <Save size={16} /> 保存配置
          </button>
          {saved && <span className="text-green-400 text-sm">已保存</span>}
        </div>
      </div>

      <div className="bg-slate-900 rounded-xl border border-slate-800 p-6">
        <h2 className="text-white font-medium mb-4">参数说明</h2>
        <table className="w-full text-sm">
          <thead><tr className="border-b border-slate-800 text-slate-400">
            <th className="text-left p-2">参数</th>
            <th className="text-left p-2">默认值</th>
            <th className="text-left p-2">说明</th>
          </tr></thead>
          <tbody className="text-slate-300">
            <tr className="border-b border-slate-800/50"><td className="p-2 font-mono">ma</td><td className="p-2">[5, 10, 20, 60]</td><td className="p-2">均线周期列表</td></tr>
            <tr className="border-b border-slate-800/50"><td className="p-2 font-mono">macd</td><td className="p-2">12/26/9</td><td className="p-2">快线/慢线/信号线周期</td></tr>
            <tr className="border-b border-slate-800/50"><td className="p-2 font-mono">kdj</td><td className="p-2">9/3/3</td><td className="p-2">RSV周期/K平滑/D平滑</td></tr>
            <tr className="border-b border-slate-800/50"><td className="p-2 font-mono">boll</td><td className="p-2">20/2.0</td><td className="p-2">MA周期/标准差倍数</td></tr>
            <tr><td className="p-2 font-mono">rsi</td><td className="p-2">[6, 14]</td><td className="p-2">RSI周期列表</td></tr>
          </tbody>
        </table>
      </div>
    </div>
  );
}

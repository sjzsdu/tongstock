import { useState } from 'react';
import { Settings, Save, Plus, X, Trash2, ChevronDown, ChevronRight, Info } from 'lucide-react';

// ── Types ──────────────────────────────────────────────────────────────────────

interface IndicatorParams {
  ma: number[];
  macd: { fast: number; slow: number; signal: number };
  kdj: { n: number; m1: number; m2: number };
  boll: { n: number; k: number };
  rsi: number[];
}

interface IndicatorConfig {
  defaults: IndicatorParams;
  categories: Record<string, Partial<IndicatorParams>>;
  overrides: Record<string, Partial<IndicatorParams>>;
}

// ── Defaults ───────────────────────────────────────────────────────────────────

const DEFAULT_PARAMS: IndicatorParams = {
  ma: [5, 10, 20, 60],
  macd: { fast: 12, slow: 26, signal: 9 },
  kdj: { n: 9, m1: 3, m2: 3 },
  boll: { n: 20, k: 2.0 },
  rsi: [6, 14],
};

const INITIAL_CONFIG: IndicatorConfig = {
  defaults: { ...DEFAULT_PARAMS, ma: [...DEFAULT_PARAMS.ma], rsi: [...DEFAULT_PARAMS.rsi] },
  categories: {
    large_cap: { ma: [5, 10, 20, 60, 120] },
    small_cap: { ma: [5, 10, 20], macd: { fast: 8, slow: 17, signal: 9 }, kdj: { n: 7, m1: 3, m2: 3 } },
  },
  overrides: {
    '000001': { kdj: { n: 5, m1: 3, m2: 3 } },
  },
};

const CATEGORY_LABELS: Record<string, string> = {
  large_cap: '大盘股',
  small_cap: '小盘股',
};

type Tab = 'defaults' | 'categories' | 'overrides';

const TABS: { key: Tab; label: string }[] = [
  { key: 'defaults', label: '默认参数' },
  { key: 'categories', label: '分类覆盖' },
  { key: 'overrides', label: '个股覆盖' },
];

// ── Tiny Components ────────────────────────────────────────────────────────────

function NumInput({ value, onChange, step, min, label }: {
  value: number; onChange: (v: number) => void; step?: number; min?: number; label: string;
}) {
  return (
    <div className="flex flex-col gap-1.5">
      <span className="text-xs text-slate-400">{label}</span>
      <input
        type="number"
        value={value}
        step={step ?? 1}
        min={min ?? 1}
        onChange={e => {
          const v = step ? parseFloat(e.target.value) : parseInt(e.target.value);
          if (!isNaN(v)) onChange(v);
        }}
        className="w-20 bg-slate-800 border border-slate-700 rounded px-2 py-1.5 text-white text-sm font-mono text-center focus:outline-none focus:border-blue-500"
      />
    </div>
  );
}

function TagInput({ values, onChange, label, description }: {
  values: number[]; onChange: (v: number[]) => void; label: string; description: string;
}) {
  const [input, setInput] = useState('');

  const add = () => {
    const v = parseInt(input);
    if (!isNaN(v) && v > 0 && !values.includes(v)) {
      onChange([...values, v].sort((a, b) => a - b));
      setInput('');
    }
  };

  return (
    <Card title={label} description={description}>
      <div className="flex flex-wrap items-center gap-2">
        {values.map(v => (
          <span key={v} className="inline-flex items-center gap-1.5 bg-blue-600/20 text-blue-400 border border-blue-500/30 rounded-full px-3 py-1 text-sm font-mono">
            {v}
            <button onClick={() => onChange(values.filter(x => x !== v))} className="text-blue-400/60 hover:text-blue-300">
              <X size={12} />
            </button>
          </span>
        ))}
        <div className="inline-flex items-center gap-1">
          <input
            type="number"
            value={input}
            onChange={e => setInput(e.target.value)}
            onKeyDown={e => e.key === 'Enter' && add()}
            placeholder="周期"
            className="w-16 bg-slate-800 border border-slate-700 rounded px-2 py-1 text-white text-sm font-mono text-center focus:outline-none focus:border-blue-500"
          />
          <button onClick={add} className="p-1 text-slate-400 hover:text-blue-400 transition-colors">
            <Plus size={14} />
          </button>
        </div>
      </div>
    </Card>
  );
}

function Card({ title, description, children, actions }: {
  title: string; description: string; children: React.ReactNode; actions?: React.ReactNode;
}) {
  return (
    <div className="bg-slate-900 rounded-lg border border-slate-800 p-4">
      <div className="flex items-start justify-between mb-3">
        <div>
          <h3 className="text-white font-medium text-sm">{title}</h3>
          <p className="text-slate-500 text-xs mt-0.5">{description}</p>
        </div>
        {actions}
      </div>
      {children}
    </div>
  );
}

// ── Indicator Param Editors ────────────────────────────────────────────────────

function MacdEditor({ value, onChange }: { value: { fast: number; slow: number; signal: number }; onChange: (v: { fast: number; slow: number; signal: number }) => void }) {
  return (
    <div className="flex items-end gap-4">
      <NumInput label="快线 (fast)" value={value.fast} onChange={v => onChange({ ...value, fast: v })} />
      <NumInput label="慢线 (slow)" value={value.slow} onChange={v => onChange({ ...value, slow: v })} />
      <NumInput label="信号线 (signal)" value={value.signal} onChange={v => onChange({ ...value, signal: v })} />
    </div>
  );
}

function KdjEditor({ value, onChange }: { value: { n: number; m1: number; m2: number }; onChange: (v: { n: number; m1: number; m2: number }) => void }) {
  return (
    <div className="flex items-end gap-4">
      <NumInput label="RSV周期 (N)" value={value.n} onChange={v => onChange({ ...value, n: v })} />
      <NumInput label="K平滑 (M1)" value={value.m1} onChange={v => onChange({ ...value, m1: v })} />
      <NumInput label="D平滑 (M2)" value={value.m2} onChange={v => onChange({ ...value, m2: v })} />
    </div>
  );
}

function BollEditor({ value, onChange }: { value: { n: number; k: number }; onChange: (v: { n: number; k: number }) => void }) {
  return (
    <div className="flex items-end gap-4">
      <NumInput label="MA周期 (N)" value={value.n} onChange={v => onChange({ ...value, n: v })} />
      <NumInput label="标准差倍数 (K)" value={value.k} onChange={v => onChange({ ...value, k: v })} step={0.1} min={0.1} />
    </div>
  );
}

function PartialParamsEditor({ params, onChange, defaults }: {
  params: Partial<IndicatorParams>; onChange: (p: Partial<IndicatorParams>) => void; defaults: IndicatorParams;
}) {
  const PARAM_OPTIONS: { key: keyof IndicatorParams; label: string }[] = [
    { key: 'ma', label: 'MA 均线' },
    { key: 'macd', label: 'MACD' },
    { key: 'kdj', label: 'KDJ' },
    { key: 'boll', label: 'BOLL' },
    { key: 'rsi', label: 'RSI' },
  ];

  const activeKeys = Object.keys(params) as (keyof IndicatorParams)[];
  const inactiveKeys = PARAM_OPTIONS.filter(o => !activeKeys.includes(o.key));

  const addParam = (key: keyof IndicatorParams) => {
    const val = structuredClone(defaults[key]);
    onChange({ ...params, [key]: val });
  };

  const removeParam = (key: keyof IndicatorParams) => {
    const next = { ...params };
    delete next[key];
    onChange(next);
  };

  return (
    <div className="space-y-3">
      {params.ma !== undefined && (
        <div className="flex items-start gap-3">
          <div className="flex-1">
            <div className="flex items-center gap-2 mb-2">
              <span className="text-xs text-slate-400 font-medium">MA 均线周期</span>
              <button onClick={() => removeParam('ma')} className="text-slate-600 hover:text-red-400"><X size={12} /></button>
            </div>
            <div className="flex flex-wrap items-center gap-2">
              {params.ma.map(v => (
                <span key={v} className="inline-flex items-center gap-1.5 bg-blue-600/20 text-blue-400 border border-blue-500/30 rounded-full px-2.5 py-0.5 text-xs font-mono">
                  {v}
                  <button onClick={() => onChange({ ...params, ma: params.ma!.filter(x => x !== v) })} className="text-blue-400/60 hover:text-blue-300"><X size={10} /></button>
                </span>
              ))}
              <InlineAddNumber onAdd={v => {
                if (!params.ma!.includes(v)) onChange({ ...params, ma: [...params.ma!, v].sort((a, b) => a - b) });
              }} />
            </div>
          </div>
        </div>
      )}

      {params.macd && (
        <div>
          <div className="flex items-center gap-2 mb-2">
            <span className="text-xs text-slate-400 font-medium">MACD</span>
            <button onClick={() => removeParam('macd')} className="text-slate-600 hover:text-red-400"><X size={12} /></button>
          </div>
          <MacdEditor value={params.macd} onChange={v => onChange({ ...params, macd: v })} />
        </div>
      )}

      {params.kdj && (
        <div>
          <div className="flex items-center gap-2 mb-2">
            <span className="text-xs text-slate-400 font-medium">KDJ</span>
            <button onClick={() => removeParam('kdj')} className="text-slate-600 hover:text-red-400"><X size={12} /></button>
          </div>
          <KdjEditor value={params.kdj} onChange={v => onChange({ ...params, kdj: v })} />
        </div>
      )}

      {params.boll && (
        <div>
          <div className="flex items-center gap-2 mb-2">
            <span className="text-xs text-slate-400 font-medium">BOLL</span>
            <button onClick={() => removeParam('boll')} className="text-slate-600 hover:text-red-400"><X size={12} /></button>
          </div>
          <BollEditor value={params.boll} onChange={v => onChange({ ...params, boll: v })} />
        </div>
      )}

      {params.rsi !== undefined && (
        <div>
          <div className="flex items-center gap-2 mb-2">
            <span className="text-xs text-slate-400 font-medium">RSI 周期</span>
            <button onClick={() => removeParam('rsi')} className="text-slate-600 hover:text-red-400"><X size={12} /></button>
          </div>
          <div className="flex flex-wrap items-center gap-2">
            {params.rsi.map(v => (
              <span key={v} className="inline-flex items-center gap-1.5 bg-blue-600/20 text-blue-400 border border-blue-500/30 rounded-full px-2.5 py-0.5 text-xs font-mono">
                {v}
                <button onClick={() => onChange({ ...params, rsi: params.rsi!.filter(x => x !== v) })} className="text-blue-400/60 hover:text-blue-300"><X size={10} /></button>
              </span>
            ))}
            <InlineAddNumber onAdd={v => {
              if (!params.rsi!.includes(v)) onChange({ ...params, rsi: [...params.rsi!, v].sort((a, b) => a - b) });
            }} />
          </div>
        </div>
      )}

      {inactiveKeys.length > 0 && (
        <div className="flex items-center gap-2 pt-1">
          <span className="text-xs text-slate-600">添加覆盖:</span>
          {inactiveKeys.map(o => (
            <button
              key={o.key}
              onClick={() => addParam(o.key)}
              className="text-xs text-slate-500 hover:text-blue-400 border border-slate-700 hover:border-blue-500/50 rounded px-2 py-0.5 transition-colors"
            >
              + {o.label}
            </button>
          ))}
        </div>
      )}
    </div>
  );
}

function InlineAddNumber({ onAdd }: { onAdd: (v: number) => void }) {
  const [input, setInput] = useState('');
  const add = () => {
    const v = parseInt(input);
    if (!isNaN(v) && v > 0) { onAdd(v); setInput(''); }
  };
  return (
    <div className="inline-flex items-center gap-1">
      <input
        type="number"
        value={input}
        onChange={e => setInput(e.target.value)}
        onKeyDown={e => e.key === 'Enter' && add()}
        placeholder="+"
        className="w-12 bg-slate-800 border border-slate-700 rounded px-1.5 py-0.5 text-white text-xs font-mono text-center focus:outline-none focus:border-blue-500"
      />
      <button onClick={add} className="text-slate-500 hover:text-blue-400"><Plus size={12} /></button>
    </div>
  );
}

// ── Collapsible Reference Table ────────────────────────────────────────────────

function ParamReference() {
  const [open, setOpen] = useState(false);
  return (
    <div className="bg-slate-900/50 rounded-lg border border-slate-800/50">
      <button onClick={() => setOpen(!open)} className="flex items-center gap-2 w-full px-4 py-2.5 text-sm text-slate-400 hover:text-slate-300 transition-colors">
        <Info size={14} />
        <span>参数说明</span>
        {open ? <ChevronDown size={14} className="ml-auto" /> : <ChevronRight size={14} className="ml-auto" />}
      </button>
      {open && (
        <div className="px-4 pb-3">
          <table className="w-full text-sm">
            <thead><tr className="border-b border-slate-800 text-slate-500">
              <th className="text-left p-2 font-normal">参数</th>
              <th className="text-left p-2 font-normal">默认值</th>
              <th className="text-left p-2 font-normal">说明</th>
            </tr></thead>
            <tbody className="text-slate-400">
              <tr className="border-b border-slate-800/30"><td className="p-2 font-mono text-xs">ma</td><td className="p-2">[5, 10, 20, 60]</td><td className="p-2">均线周期列表</td></tr>
              <tr className="border-b border-slate-800/30"><td className="p-2 font-mono text-xs">macd</td><td className="p-2">12 / 26 / 9</td><td className="p-2">快线 / 慢线 / 信号线周期</td></tr>
              <tr className="border-b border-slate-800/30"><td className="p-2 font-mono text-xs">kdj</td><td className="p-2">9 / 3 / 3</td><td className="p-2">RSV周期 / K平滑 / D平滑</td></tr>
              <tr className="border-b border-slate-800/30"><td className="p-2 font-mono text-xs">boll</td><td className="p-2">20 / 2.0</td><td className="p-2">MA周期 / 标准差倍数</td></tr>
              <tr><td className="p-2 font-mono text-xs">rsi</td><td className="p-2">[6, 14]</td><td className="p-2">RSI周期列表</td></tr>
            </tbody>
          </table>
        </div>
      )}
    </div>
  );
}

// ── Tab: Defaults ──────────────────────────────────────────────────────────────

function DefaultsTab({ config, onChange }: { config: IndicatorConfig; onChange: (c: IndicatorConfig) => void }) {
  const d = config.defaults;
  const set = (patch: Partial<IndicatorParams>) => onChange({ ...config, defaults: { ...d, ...patch } });

  return (
    <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
      <TagInput label="MA 均线" description="移动平均线周期列表" values={d.ma} onChange={ma => set({ ma })} />
      <TagInput label="RSI" description="相对强弱指标周期列表" values={d.rsi} onChange={rsi => set({ rsi })} />
      <Card title="MACD" description="指数平滑异同移动平均线">
        <MacdEditor value={d.macd} onChange={macd => set({ macd })} />
      </Card>
      <Card title="KDJ" description="随机指标">
        <KdjEditor value={d.kdj} onChange={kdj => set({ kdj })} />
      </Card>
      <Card title="BOLL" description="布林带通道">
        <BollEditor value={d.boll} onChange={boll => set({ boll })} />
      </Card>
    </div>
  );
}

// ── Tab: Categories ────────────────────────────────────────────────────────────

function CategoriesTab({ config, onChange }: { config: IndicatorConfig; onChange: (c: IndicatorConfig) => void }) {
  const [expanded, setExpanded] = useState<Record<string, boolean>>(() => {
    const init: Record<string, boolean> = {};
    Object.keys(config.categories).forEach(k => { init[k] = true; });
    return init;
  });
  const [newName, setNewName] = useState('');
  const [adding, setAdding] = useState(false);

  const toggle = (key: string) => setExpanded(prev => ({ ...prev, [key]: !prev[key] }));

  const remove = (key: string) => {
    const next = { ...config.categories };
    delete next[key];
    onChange({ ...config, categories: next });
  };

  const add = () => {
    const name = newName.trim();
    if (!name || config.categories[name]) return;
    onChange({ ...config, categories: { ...config.categories, [name]: {} } });
    setExpanded(prev => ({ ...prev, [name]: true }));
    setNewName('');
    setAdding(false);
  };

  const updateCat = (key: string, params: Partial<IndicatorParams>) => {
    onChange({ ...config, categories: { ...config.categories, [key]: params } });
  };

  const entries = Object.entries(config.categories);

  return (
    <div className="space-y-4">
      <p className="text-slate-500 text-sm">按市值分类覆盖默认参数，优先级：个股覆盖 &gt; 分类覆盖 &gt; 默认参数</p>

      {entries.map(([key, params]) => (
        <div key={key} className="bg-slate-900 rounded-lg border border-slate-800">
          <button
            onClick={() => toggle(key)}
            className="flex items-center gap-2 w-full px-4 py-3 text-left hover:bg-slate-800/50 transition-colors rounded-t-lg"
          >
            {expanded[key] ? <ChevronDown size={14} className="text-slate-400" /> : <ChevronRight size={14} className="text-slate-400" />}
            <span className="text-white font-medium text-sm">{CATEGORY_LABELS[key] || key}</span>
            <span className="text-slate-600 text-xs font-mono">{key}</span>
            <span className="text-slate-600 text-xs ml-auto mr-2">{Object.keys(params).length} 项覆盖</span>
            <button
              onClick={e => { e.stopPropagation(); remove(key); }}
              className="text-slate-600 hover:text-red-400 transition-colors p-1"
            >
              <Trash2 size={14} />
            </button>
          </button>
          {expanded[key] && (
            <div className="px-4 pb-4 border-t border-slate-800">
              <div className="pt-3">
                <PartialParamsEditor params={params} onChange={p => updateCat(key, p)} defaults={config.defaults} />
              </div>
            </div>
          )}
        </div>
      ))}

      {adding ? (
        <div className="flex items-center gap-2">
          <input
            type="text"
            value={newName}
            onChange={e => setNewName(e.target.value)}
            onKeyDown={e => e.key === 'Enter' && add()}
            placeholder="分类名称 (如 mid_cap)"
            autoFocus
            className="bg-slate-800 border border-slate-700 rounded px-3 py-1.5 text-white text-sm font-mono focus:outline-none focus:border-blue-500 w-56"
          />
          <button onClick={add} className="text-sm text-blue-400 hover:text-blue-300 px-2 py-1">确定</button>
          <button onClick={() => { setAdding(false); setNewName(''); }} className="text-sm text-slate-500 hover:text-slate-300 px-2 py-1">取消</button>
        </div>
      ) : (
        <button
          onClick={() => setAdding(true)}
          className="flex items-center gap-2 text-sm text-slate-400 hover:text-blue-400 border border-dashed border-slate-700 hover:border-blue-500/50 rounded-lg px-4 py-2.5 w-full justify-center transition-colors"
        >
          <Plus size={14} /> 添加分类
        </button>
      )}
    </div>
  );
}

// ── Tab: Overrides ─────────────────────────────────────────────────────────────

function OverridesTab({ config, onChange }: { config: IndicatorConfig; onChange: (c: IndicatorConfig) => void }) {
  const [expanded, setExpanded] = useState<Record<string, boolean>>(() => {
    const init: Record<string, boolean> = {};
    Object.keys(config.overrides).forEach(k => { init[k] = true; });
    return init;
  });
  const [newCode, setNewCode] = useState('');
  const [adding, setAdding] = useState(false);

  const toggle = (key: string) => setExpanded(prev => ({ ...prev, [key]: !prev[key] }));

  const remove = (key: string) => {
    const next = { ...config.overrides };
    delete next[key];
    onChange({ ...config, overrides: next });
  };

  const add = () => {
    const code = newCode.trim();
    if (!/^\d{6}$/.test(code) || config.overrides[code]) return;
    onChange({ ...config, overrides: { ...config.overrides, [code]: {} } });
    setExpanded(prev => ({ ...prev, [code]: true }));
    setNewCode('');
    setAdding(false);
  };

  const updateOverride = (key: string, params: Partial<IndicatorParams>) => {
    onChange({ ...config, overrides: { ...config.overrides, [key]: params } });
  };

  const entries = Object.entries(config.overrides);

  return (
    <div className="space-y-4">
      <p className="text-slate-500 text-sm">为特定股票设置专属参数，优先级最高</p>

      {entries.map(([code, params]) => (
        <div key={code} className="bg-slate-900 rounded-lg border border-slate-800">
          <button
            onClick={() => toggle(code)}
            className="flex items-center gap-2 w-full px-4 py-3 text-left hover:bg-slate-800/50 transition-colors rounded-t-lg"
          >
            {expanded[code] ? <ChevronDown size={14} className="text-slate-400" /> : <ChevronRight size={14} className="text-slate-400" />}
            <span className="text-white font-medium text-sm font-mono">{code}</span>
            <span className="text-slate-600 text-xs ml-auto mr-2">{Object.keys(params).length} 项覆盖</span>
            <button
              onClick={e => { e.stopPropagation(); remove(code); }}
              className="text-slate-600 hover:text-red-400 transition-colors p-1"
            >
              <Trash2 size={14} />
            </button>
          </button>
          {expanded[code] && (
            <div className="px-4 pb-4 border-t border-slate-800">
              <div className="pt-3">
                <PartialParamsEditor params={params} onChange={p => updateOverride(code, p)} defaults={config.defaults} />
              </div>
            </div>
          )}
        </div>
      ))}

      {adding ? (
        <div className="flex items-center gap-2">
          <input
            type="text"
            value={newCode}
            onChange={e => setNewCode(e.target.value.replace(/\D/g, '').slice(0, 6))}
            onKeyDown={e => e.key === 'Enter' && add()}
            placeholder="6位股票代码"
            autoFocus
            className="bg-slate-800 border border-slate-700 rounded px-3 py-1.5 text-white text-sm font-mono focus:outline-none focus:border-blue-500 w-40"
          />
          <button onClick={add} disabled={!/^\d{6}$/.test(newCode)} className="text-sm text-blue-400 hover:text-blue-300 px-2 py-1 disabled:text-slate-600 disabled:cursor-not-allowed">确定</button>
          <button onClick={() => { setAdding(false); setNewCode(''); }} className="text-sm text-slate-500 hover:text-slate-300 px-2 py-1">取消</button>
        </div>
      ) : (
        <button
          onClick={() => setAdding(true)}
          className="flex items-center gap-2 text-sm text-slate-400 hover:text-blue-400 border border-dashed border-slate-700 hover:border-blue-500/50 rounded-lg px-4 py-2.5 w-full justify-center transition-colors"
        >
          <Plus size={14} /> 添加个股覆盖
        </button>
      )}
    </div>
  );
}

// ── Main Page ──────────────────────────────────────────────────────────────────

export default function SettingsPage() {
  const [config, setConfig] = useState<IndicatorConfig>(structuredClone(INITIAL_CONFIG));
  const [tab, setTab] = useState<Tab>('defaults');
  const [saved, setSaved] = useState(false);

  const handleSave = () => {
    // TODO: POST to API when backend is ready
    console.log('Save config:', JSON.stringify(config, null, 2));
    setSaved(true);
    setTimeout(() => setSaved(false), 2000);
  };

  return (
    <div className="space-y-5 max-w-4xl">
      <div>
        <h1 className="text-2xl font-bold text-white flex items-center gap-2">
          <Settings size={24} /> 配置
        </h1>
        <p className="text-slate-500 text-sm mt-1">指标参数配置 · <span className="font-mono">~/.tongstock/indicator.yaml</span></p>
      </div>

      <div className="flex items-center gap-1 border-b border-slate-800">
        {TABS.map(t => (
          <button
            key={t.key}
            onClick={() => setTab(t.key)}
            className={`px-4 py-2 text-sm transition-colors ${
              tab === t.key
                ? 'text-white border-b-2 border-blue-500 bg-slate-800 rounded-t-lg'
                : 'text-slate-400 hover:text-white'
            }`}
          >
            {t.label}
          </button>
        ))}
      </div>

      {tab === 'defaults' && <DefaultsTab config={config} onChange={setConfig} />}
      {tab === 'categories' && <CategoriesTab config={config} onChange={setConfig} />}
      {tab === 'overrides' && <OverridesTab config={config} onChange={setConfig} />}

      <ParamReference />

      <div className="flex items-center gap-4">
        <button
          onClick={handleSave}
          className="flex items-center gap-2 bg-blue-600 hover:bg-blue-700 px-4 py-2 rounded-lg text-white font-medium transition-colors"
        >
          <Save size={16} /> 保存配置
        </button>
        {saved && <span className="text-green-400 text-sm animate-pulse">已保存</span>}
      </div>
    </div>
  );
}

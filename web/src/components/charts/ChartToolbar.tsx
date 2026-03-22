interface Props {
  ktype: string;
  onKtypeChange: (ktype: string) => void;
  mainOverlay: string;
  onMainOverlayChange: (v: string) => void;
  subPanel: string;
  onSubPanelChange: (v: string) => void;
}

const KTYPES = [
  { value: '1m', label: '1分' },
  { value: '5m', label: '5分' },
  { value: '15m', label: '15分' },
  { value: '30m', label: '30分' },
  { value: '60m', label: '60分' },
  { value: 'day', label: '日K' },
  { value: 'week', label: '周K' },
  { value: 'month', label: '月K' },
];

const MAIN_OVERLAYS = [
  { value: 'MA', label: 'MA' },
  { value: 'BOLL', label: 'BOLL' },
];

const SUB_PANELS = [
  { value: 'MACD', label: 'MACD' },
  { value: 'KDJ', label: 'KDJ' },
  { value: 'RSI', label: 'RSI' },
];

export default function ChartToolbar({ ktype, onKtypeChange, mainOverlay, onMainOverlayChange, subPanel, onSubPanelChange }: Props) {
  return (
    <div className="flex items-center justify-between bg-slate-900 rounded-t-lg border border-slate-800 border-b-0 px-3 py-2">
      <div className="flex gap-0.5">
        {KTYPES.map(k => (
          <button
            key={k.value}
            onClick={() => onKtypeChange(k.value)}
            className={`px-2.5 py-1 rounded text-xs font-medium transition-colors ${
              ktype === k.value ? 'bg-blue-600 text-white' : 'text-slate-400 hover:text-white hover:bg-slate-800'
            }`}
          >
            {k.label}
          </button>
        ))}
      </div>
      <div className="flex gap-3">
        <div className="flex gap-0.5 items-center">
          <span className="text-slate-500 text-xs mr-1">主图</span>
          {MAIN_OVERLAYS.map(o => (
            <button
              key={o.value}
              onClick={() => onMainOverlayChange(mainOverlay === o.value ? '' : o.value)}
              className={`px-2.5 py-1 rounded text-xs font-medium transition-colors ${
                mainOverlay === o.value ? 'bg-yellow-600 text-white' : 'text-slate-500 hover:text-slate-300 hover:bg-slate-800'
              }`}
            >
              {o.label}
            </button>
          ))}
        </div>
        <div className="flex gap-0.5 items-center">
          <span className="text-slate-500 text-xs mr-1">副图</span>
          {SUB_PANELS.map(s => (
            <button
              key={s.value}
              onClick={() => onSubPanelChange(subPanel === s.value ? '' : s.value)}
              className={`px-2.5 py-1 rounded text-xs font-medium transition-colors ${
                subPanel === s.value ? 'bg-blue-700 text-white' : 'text-slate-500 hover:text-slate-300 hover:bg-slate-800'
              }`}
            >
              {s.label}
            </button>
          ))}
        </div>
      </div>
    </div>
  );
}

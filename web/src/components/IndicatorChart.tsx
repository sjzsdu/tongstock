import { ResponsiveContainer, ComposedChart, Line, Bar, XAxis, YAxis, Tooltip, CartesianGrid } from 'recharts';
import type { IndicatorData } from '../types/api';

interface Props {
  data: IndicatorData;
}

export default function IndicatorChart({ data }: Props) {
  const n = data.ma?.['5']?.length || 0;
  const show = Math.min(n, 60);
  const start = n - show;

  const chartData = Array.from({ length: show }, (_, i) => {
    const idx = start + i;
    return {
      idx: i,
      ma5: data.ma?.['5']?.[idx] ?? null,
      ma10: data.ma?.['10']?.[idx] ?? null,
      ma20: data.ma?.['20']?.[idx] ?? null,
      dif: data.macd?.DIF?.[idx] ?? null,
      dea: data.macd?.DEA?.[idx] ?? null,
      hist: data.macd?.Hist?.[idx] ?? null,
      k: data.kdj?.K?.[idx] ?? null,
      d: data.kdj?.D?.[idx] ?? null,
      j: data.kdj?.J?.[idx] ?? null,
      upper: data.boll?.Upper?.[idx] ?? null,
      middle: data.boll?.Middle?.[idx] ?? null,
      lower: data.boll?.Lower?.[idx] ?? null,
    };
  });

  return (
    <div className="space-y-4">
      <div className="bg-slate-900 rounded-xl border border-slate-800 p-4">
        <h3 className="text-white font-medium mb-3">MA 均线</h3>
        <ResponsiveContainer width="100%" height={200}>
          <ComposedChart data={chartData}>
            <CartesianGrid strokeDasharray="3 3" stroke="#334155" />
            <XAxis dataKey="idx" tick={{ fill: '#94a3b8', fontSize: 10 }} />
            <YAxis tick={{ fill: '#94a3b8', fontSize: 10 }} domain={['auto', 'auto']} />
            <Tooltip contentStyle={{ background: '#1e293b', border: '1px solid #334155', borderRadius: 8, color: '#e2e8f0' }} />
            <Line type="monotone" dataKey="ma5" stroke="#f59e0b" dot={false} name="MA5" />
            <Line type="monotone" dataKey="ma10" stroke="#3b82f6" dot={false} name="MA10" />
            <Line type="monotone" dataKey="ma20" stroke="#8b5cf6" dot={false} name="MA20" />
          </ComposedChart>
        </ResponsiveContainer>
      </div>

      {data.macd && (
        <div className="bg-slate-900 rounded-xl border border-slate-800 p-4">
          <h3 className="text-white font-medium mb-3">MACD</h3>
          <ResponsiveContainer width="100%" height={200}>
            <ComposedChart data={chartData}>
              <CartesianGrid strokeDasharray="3 3" stroke="#334155" />
              <XAxis dataKey="idx" tick={{ fill: '#94a3b8', fontSize: 10 }} />
              <YAxis tick={{ fill: '#94a3b8', fontSize: 10 }} />
              <Tooltip contentStyle={{ background: '#1e293b', border: '1px solid #334155', borderRadius: 8, color: '#e2e8f0' }} />
              <Line type="monotone" dataKey="dif" stroke="#f59e0b" dot={false} name="DIF" />
              <Line type="monotone" dataKey="dea" stroke="#3b82f6" dot={false} name="DEA" />
              <Bar dataKey="hist" fill="#22c55e" name="Histogram" shape={(props: any) => {
                const { x, y, width, height } = props;
                const color = props.value >= 0 ? '#ef4444' : '#22c55e';
                return <rect x={x} y={y} width={width} height={height} fill={color} />;
              }} />
            </ComposedChart>
          </ResponsiveContainer>
        </div>
      )}

      {data.kdj && (
        <div className="bg-slate-900 rounded-xl border border-slate-800 p-4">
          <h3 className="text-white font-medium mb-3">KDJ</h3>
          <ResponsiveContainer width="100%" height={200}>
            <ComposedChart data={chartData}>
              <CartesianGrid strokeDasharray="3 3" stroke="#334155" />
              <XAxis dataKey="idx" tick={{ fill: '#94a3b8', fontSize: 10 }} />
              <YAxis tick={{ fill: '#94a3b8', fontSize: 10 }} />
              <Tooltip contentStyle={{ background: '#1e293b', border: '1px solid #334155', borderRadius: 8, color: '#e2e8f0' }} />
              <Line type="monotone" dataKey="k" stroke="#f59e0b" dot={false} name="K" />
              <Line type="monotone" dataKey="d" stroke="#3b82f6" dot={false} name="D" />
              <Line type="monotone" dataKey="j" stroke="#ef4444" dot={false} name="J" />
            </ComposedChart>
          </ResponsiveContainer>
        </div>
      )}

      {data.boll && (
        <div className="bg-slate-900 rounded-xl border border-slate-800 p-4">
          <h3 className="text-white font-medium mb-3">BOLL</h3>
          <ResponsiveContainer width="100%" height={200}>
            <ComposedChart data={chartData}>
              <CartesianGrid strokeDasharray="3 3" stroke="#334155" />
              <XAxis dataKey="idx" tick={{ fill: '#94a3b8', fontSize: 10 }} />
              <YAxis tick={{ fill: '#94a3b8', fontSize: 10 }} domain={['auto', 'auto']} />
              <Tooltip contentStyle={{ background: '#1e293b', border: '1px solid #334155', borderRadius: 8, color: '#e2e8f0' }} />
              <Line type="monotone" dataKey="upper" stroke="#ef4444" dot={false} name="Upper" />
              <Line type="monotone" dataKey="middle" stroke="#f59e0b" dot={false} name="Middle" />
              <Line type="monotone" dataKey="lower" stroke="#22c55e" dot={false} name="Lower" />
            </ComposedChart>
          </ResponsiveContainer>
        </div>
      )}
    </div>
  );
}

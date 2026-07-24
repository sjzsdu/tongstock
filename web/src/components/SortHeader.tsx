import { ArrowDownOutlined, ArrowUpOutlined } from '@ant-design/icons';
import type { ReactNode } from 'react';

interface SortHeaderProps<K extends string> {
  sortKey: K;
  sortAsc: boolean;
  current: K;
  onChange: (key: K) => void;
  align?: 'left' | 'right';
  children: ReactNode;
}

export function SortHeader<K extends string>({
  sortKey,
  sortAsc,
  current,
  onChange,
  align = 'left',
  children,
}: SortHeaderProps<K>) {
  const active = current === sortKey;

  return (
    <button
      type="button"
      onClick={() => onChange(sortKey)}
      style={{
        display: 'flex',
        alignItems: 'center',
        justifyContent: align === 'right' ? 'flex-end' : 'flex-start',
        gap: 4,
        width: '100%',
        border: 'none',
        background: 'transparent',
        color: active ? 'var(--ant-color-text)' : 'var(--ant-color-text-secondary)',
        fontSize: 12,
        cursor: 'pointer',
      }}
    >
      <span>{children}</span>
      {active ? (sortAsc ? <ArrowUpOutlined /> : <ArrowDownOutlined />) : <span style={{ opacity: 0.35 }}>↕</span>}
    </button>
  );
}
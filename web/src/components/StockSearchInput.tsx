import { useEffect, useMemo, useRef, useState } from 'react';
import type { ReactNode } from 'react';
import { AutoComplete, Input, Spin, Tag, Typography } from 'antd';
import { SearchOutlined } from '@ant-design/icons';
import type { SearchStockMatch } from '../types/api';
import { getStockSearchIndex, maybeRefreshStockSearchIndex, searchCachedStocks } from '../services/stockSearchCache';

interface StockSearchInputProps {
  initialQuery?: string;
  placeholder?: string;
  limit?: number;
  autoFocus?: boolean;
  panelClassName?: string;
  inputClassName?: string;
  containerClassName?: string;
  iconClassName?: string;
  showIcon?: boolean;
  emptyText?: string;
  /** 选中后是否清空输入框，适合连续添加的表单场景（如自选股）。 */
  clearOnSelect?: boolean;
  onSelect: (match: SearchStockMatch) => void;
}

export default function StockSearchInput({
  initialQuery = '',
  placeholder = '输入股票代码、名称或拼音',
  limit = 10,
  autoFocus = false,
  containerClassName = '',
  emptyText,
  clearOnSelect = false,
  onSelect,
}: StockSearchInputProps) {
  const [query, setQuery] = useState(initialQuery);
  const [results, setResults] = useState<SearchStockMatch[]>([]);
  const [loading, setLoading] = useState(false);
  const [message, setMessage] = useState<string | null>(null);
  const requestSeq = useRef(0);

  useEffect(() => {
    maybeRefreshStockSearchIndex();
  }, []);

  useEffect(() => {
    setQuery(initialQuery);
  }, [initialQuery]);

  useEffect(() => {
    const trimmed = query.trim();
    if (!trimmed) {
      setResults([]);
      setMessage(null);
      setLoading(false);
      return;
    }

    const seq = ++requestSeq.current;
    const timer = window.setTimeout(async () => {
      setLoading(false);
      try {
        const cache = await getStockSearchIndex();
        if (requestSeq.current !== seq) return;
        const response = searchCachedStocks(trimmed, cache.items, limit);

        setResults(response.matches);
        if (response.matches.length === 0) {
          setMessage(emptyText ?? `未找到股票 “${trimmed}”`);
        } else if (!response.resolved && response.matches.length > 1) {
          setMessage(`找到 ${response.total} 个匹配项，请选择具体个股`);
        } else {
          setMessage(null);
        }
      } catch {
        if (requestSeq.current !== seq) return;
        setResults([]);
        setMessage('搜索失败，请稍后重试');
      } finally {
        if (requestSeq.current === seq) {
          setLoading(false);
        }
      }
    }, 150);

    return () => window.clearTimeout(timer);
  }, [emptyText, limit, query]);

  const options = useMemo(() => {
    const mapped: Array<{ value: string; label: ReactNode; match?: SearchStockMatch }> = results.map((match) => ({
      value: match.name,
      label: (
        <div
          style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', gap: 12 }}
          onMouseDown={(event) => event.preventDefault()}
        >
          <div style={{ minWidth: 0 }}>
            <div style={{ fontWeight: 600 }}>{match.name}</div>
            <Typography.Text type="secondary" style={{ fontSize: 12 }}>
              {match.exchange} · {labelForMatchType(match.matchType)}
            </Typography.Text>
          </div>
          <Tag color="blue" style={{ marginInlineEnd: 0, fontFamily: 'monospace' }}>{match.code}</Tag>
        </div>
      ),
      match,
    }));

    if (message) {
      mapped.push({
        value: '__message__',
        label: (
          <Typography.Text type="secondary" style={{ display: 'block', padding: '4px 0' }}>
            {message}
          </Typography.Text>
        ),
        match: undefined,
      });
    }

    return mapped;
  }, [message, results]);

  const handleEnter = () => {
    if (results.length === 1) {
      onSelect(results[0]);
      setQuery(clearOnSelect ? '' : results[0].name);
    }
  };

  return (
    <div className={containerClassName}>
      <AutoComplete
        value={query}
        options={options}
        onSearch={setQuery}
        onChange={setQuery}
        onSelect={(_, option) => {
          const match = (option as { match?: SearchStockMatch }).match;
          if (match) {
            // 清空而非回填名称：连续添加时省一次手动删除，
            // 残留的名称也会误导下一次搜索。
            setQuery(clearOnSelect ? '' : match.name);
            onSelect(match);
          }
        }}
        style={{ width: '100%' }}
        notFoundContent={loading ? <Spin size="small" /> : null}
        filterOption={false}
      >
        <Input
          allowClear
          autoFocus={autoFocus}
          placeholder={placeholder}
          prefix={<SearchOutlined />}
          onPressEnter={handleEnter}
          suffix={loading ? <Spin size="small" /> : null}
        />
      </AutoComplete>
    </div>
  );
}

function labelForMatchType(matchType: string): string {
  switch (matchType) {
    case 'exact_code':
      return '代码精确匹配';
    case 'exact_name':
      return '名称精确匹配';
    case 'exact_pinyin':
      return '拼音精确匹配';
    case 'exact_initials':
      return '字母精确匹配';
    case 'prefix_code':
      return '代码前缀匹配';
    case 'prefix_name':
      return '名称前缀匹配';
    case 'prefix_pinyin':
      return '拼音前缀匹配';
    case 'prefix_initials':
      return '字母前缀匹配';
    case 'contains_code':
      return '代码包含匹配';
    case 'contains_name':
      return '名称包含匹配';
    case 'contains_pinyin':
      return '拼音包含匹配';
    case 'contains_initials':
      return '字母包含匹配';
    default:
      return '匹配结果';
  }
}

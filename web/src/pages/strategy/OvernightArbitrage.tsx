import { useState, useEffect, useCallback, useRef, useMemo } from 'react';
import { useNavigate } from 'react-router-dom';
import {
	ArrowDownOutlined,
	ArrowUpOutlined,
	CloseOutlined,
	EditOutlined,
	InfoCircleOutlined,
	PlusOutlined,
	SearchOutlined,
	ShoppingCartOutlined,
	MinusCircleOutlined,
} from '@ant-design/icons';
import { useVirtualizer } from '@tanstack/react-virtual';
import {
	Alert,
	Button,
	Card,
	Collapse,
	Empty,
	Flex,
	Input,
	List,
	Modal,
	Segmented,
	Space,
	Spin,
	Statistic,
	Tag,
	Typography,
	message,
} from 'antd';
import { api, type TradeInfo, type OvernightCandidate } from '../../api/client';

const { Paragraph, Text, Title } = Typography;

const ROW_HEIGHT = 48;

type SourceTab = 'watchlist' | 'block';
type SortKey = 'code' | 'name' | 'price' | 'change_pct';

interface StockItem {
	code: string;
	name?: string;
}

interface BlockInfo {
	name: string;
	type: number;
	count: number;
	stocks?: string[];
	stocksWithNames?: { code: string; name: string }[];
}

type BlockListItem = { name: string; type: number; count: number };

type CodesCacheEntry = { list: { Code?: string; Name?: string }[]; timestamp: number };

function formatPercent(value: number): string {
	return `${value > 0 ? '+' : ''}${value.toFixed(2)}%`;
}

function getPriceColor(value: number): string {
	if (value > 0) return 'var(--ant-color-error)';
	if (value < 0) return 'var(--ant-color-success)';
	return 'var(--ant-color-text-secondary)';
}

function SortHeader({
	sortKey,
	sortAsc,
	current,
	onChange,
	align = 'left',
	children,
}: {
	sortKey: SortKey;
	sortAsc: boolean;
	current: SortKey;
	onChange: (key: SortKey) => void;
	align?: 'left' | 'right';
	children: React.ReactNode;
}) {
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

function VirtualResultTable({
	results,
	tableContainerRef,
	sortKey,
	sortAsc,
	onSortChange,
	navigate,
	trades,
	tradingLoading,
	handleBuy,
	handleSell,
}: {
	results: OvernightCandidate[];
	tableContainerRef: React.RefObject<HTMLDivElement | null>;
	sortKey: SortKey;
	sortAsc: boolean;
	onSortChange: (key: SortKey) => void;
	navigate: (path: string) => void;
	trades: Record<string, TradeInfo>;
	tradingLoading: boolean;
	handleBuy: (result: OvernightCandidate) => void;
	handleSell: (result: OvernightCandidate) => void;
}) {
	const rowVirtualizer = useVirtualizer({
		count: results.length,
		getScrollElement: () => tableContainerRef.current,
		estimateSize: () => ROW_HEIGHT,
		overscan: 18,
	});

	return (
		<Card bodyStyle={{ padding: 0 }}>
			<div
				style={{
					display: 'grid',
					gridTemplateColumns: '80px 1fr 96px 96px 80px 80px 1fr 140px',
					gap: 0,
					padding: '0 16px',
					borderBottom: '1px solid var(--ant-color-border-secondary)',
					background: 'var(--ant-color-fill-quaternary)',
				}}
			>
				<div style={{ padding: '10px 0' }}><SortHeader sortKey="code" sortAsc={sortAsc} current={sortKey} onChange={onSortChange}>代码</SortHeader></div>
				<div style={{ padding: '10px 12px' }}><SortHeader sortKey="name" sortAsc={sortAsc} current={sortKey} onChange={onSortChange}>名称</SortHeader></div>
				<div style={{ padding: '10px 0' }}><SortHeader sortKey="price" sortAsc={sortAsc} current={sortKey} onChange={onSortChange} align="right">价格</SortHeader></div>
				<div style={{ padding: '10px 0' }}><SortHeader sortKey="change_pct" sortAsc={sortAsc} current={sortKey} onChange={onSortChange} align="right">涨幅</SortHeader></div>
				<div style={{ padding: '10px 0', textAlign: 'right', color: 'var(--ant-color-text-secondary)', fontSize: 12 }}>量比</div>
				<div style={{ padding: '10px 0', textAlign: 'right', color: 'var(--ant-color-text-secondary)', fontSize: 12 }}>换手</div>
				<div style={{ padding: '10px 12px', color: 'var(--ant-color-text-secondary)', fontSize: 12 }}>选股标准</div>
				<div style={{ padding: '10px 0', textAlign: 'center', color: 'var(--ant-color-text-secondary)', fontSize: 12 }}>虚拟操作</div>
			</div>

			<div ref={tableContainerRef} style={{ maxHeight: 'calc(100vh - 480px)', minHeight: 320, overflow: 'auto' }}>
				<div style={{ height: rowVirtualizer.getTotalSize(), position: 'relative' }}>
					{rowVirtualizer.getVirtualItems().map((virtualRow) => {
						const result = results[virtualRow.index];
						const criteria = result.criteria;
						const criteriaKeys = Object.keys(criteria) as (keyof typeof criteria)[];

						return (
							<div
								key={result.code}
								onClick={() => navigate(`/stock/${result.code}/chart`)}
								style={{
									position: 'absolute',
									top: virtualRow.start,
									left: 0,
									width: '100%',
									height: ROW_HEIGHT,
									padding: '0 16px',
									display: 'grid',
									gridTemplateColumns: '80px 1fr 96px 96px 80px 80px 1fr 140px',
									alignItems: 'center',
									borderBottom: '1px solid var(--ant-color-border-secondary)',
									cursor: 'pointer',
									background: virtualRow.index % 2 === 0 ? 'transparent' : 'var(--ant-color-fill-quaternary)',
								}}
							>
								<Text code>{result.code}</Text>
								<Text ellipsis style={{ padding: '0 12px' }}>{result.name || '-'}</Text>
								<Text style={{ textAlign: 'right', fontVariantNumeric: 'tabular-nums' }}>{result.price.toFixed(2)}</Text>
								<Text style={{ textAlign: 'right', color: getPriceColor(result.change_pct), fontVariantNumeric: 'tabular-nums' }}>{formatPercent(result.change_pct)}</Text>
								<Text style={{ textAlign: 'right', fontVariantNumeric: 'tabular-nums' }}>{result.volume_ratio.toFixed(2)}</Text>
								<Text style={{ textAlign: 'right', fontVariantNumeric: 'tabular-nums' }}>{result.turnover_rate.toFixed(2)}%</Text>
								<div style={{ paddingLeft: 12 }}>
									<Space size={[4, 0]} wrap>
										{criteriaKeys.map((key) => (
											<Tag key={key} color={criteria[key] ? 'green' : 'red'} style={{ fontSize: 10 }}>
												{criteria[key] ? '✓' : '✗'}
											</Tag>
										))}
									</Space>
								</div>
								<div style={{ textAlign: 'center' }}>
									<Space size={4}>
										<Button
											size="small"
											type="primary"
											icon={<ShoppingCartOutlined />}
											onClick={(e) => { e.stopPropagation(); handleBuy(result); }}
											disabled={tradingLoading || (trades[result.code]?.action === 'buy')}
											style={{ padding: '4px 8px', fontSize: 11 }}
										>
											买入
										</Button>
										<Button
											size="small"
											danger
											icon={<MinusCircleOutlined />}
											onClick={(e) => { e.stopPropagation(); handleSell(result); }}
											disabled={tradingLoading || (!trades[result.code] || trades[result.code].action !== 'buy')}
											style={{ padding: '4px 8px', fontSize: 11 }}
										>
											卖出
										</Button>
									</Space>
									{trades[result.code] && (
										<div style={{ fontSize: 10, marginTop: 2, color: trades[result.code].action === 'buy' ? '#ff4d4f' : '#52c41a' }}>
											{trades[result.code].action === 'buy' ? `已买入@${trades[result.code].price.toFixed(2)}` : '已卖出'}
										</div>
									)}
								</div>
							</div>
						);
					})}
				</div>
			</div>
		</Card>
	);
}

export default function OvernightArbitrage() {
	const navigate = useNavigate();
	const tableContainerRef = useRef<HTMLDivElement>(null);
	const [messageApi, contextHolder] = message.useMessage();

	const STORAGE_KEY = 'tongstock_stocklist';
	const CACHE_EXPIRY = 5 * 60 * 1000;

	const loadStockListFromStorage = useCallback((): StockItem[] => {
		try {
			const stored = localStorage.getItem(STORAGE_KEY);
			return stored ? JSON.parse(stored) : [];
		} catch {
			return [];
		}
	}, []);

	const saveStockListToStorage = useCallback((list: StockItem[]) => {
		try {
			localStorage.setItem(STORAGE_KEY, JSON.stringify(list));
		} catch {
			return;
		}
	}, []);

	const [codesCache, setCodesCache] = useState<Record<string, CodesCacheEntry>>({});
	const [sourceTab, setSourceTab] = useState<SourceTab>('watchlist');
	const [stockList, setStockList] = useState<StockItem[]>(() => loadStockListFromStorage());
	const [inputCode, setInputCode] = useState('');
	const [inputLoading, setInputLoading] = useState(false);
	const [results, setResults] = useState<OvernightCandidate[]>([]);
	const [failedCodes, setFailedCodes] = useState<{ code: string; reason: string }[]>([]);
	const [hasScreenLoaded, setHasScreenLoaded] = useState(false);
	const [total, setTotal] = useState(0);
	const [loading, setLoading] = useState(false);
	const [sortKey, setSortKey] = useState<SortKey>('change_pct');
	const [sortAsc, setSortAsc] = useState(false);
	const [blockFile, setBlockFile] = useState('block_zs.dat');
	const [blockData, setBlockData] = useState<BlockListItem[]>([]);
	const [selectedBlock, setSelectedBlock] = useState<BlockInfo | null>(null);
	const [blockLoading, setBlockLoading] = useState(false);
	const [blockStocksLoading, setBlockStocksLoading] = useState(false);
	const [blockSearch, setBlockSearch] = useState('');
	const [showBlockModal, setShowBlockModal] = useState(false);
	const [showSourceModal, setShowSourceModal] = useState(false);
	const [blockStocksWithNames, setBlockStocksWithNames] = useState<{ code: string; name: string }[]>([]);
	const [blockStocksLoadingNames, setBlockStocksLoadingNames] = useState(false);
	const [isOvernightTime, setIsOvernightTime] = useState(false);
	const [currentTime, setCurrentTime] = useState('');

	const [trades, setTrades] = useState<Record<string, TradeInfo>>({});
	const [tradingLoading, setTradingLoading] = useState(false);
	const [showTradeModal, setShowTradeModal] = useState(false);
	const [currentTradeAction, setCurrentTradeAction] = useState<'buy' | 'sell'>('buy');
	const [currentTradeStock, setCurrentTradeStock] = useState<OvernightCandidate | null>(null);
	const [tradeReason, setTradeReason] = useState('');

	useEffect(() => {
		saveStockListToStorage(stockList);
	}, [stockList, saveStockListToStorage]);

	const loadTrades = useCallback(async (codes: string[]) => {
		if (codes.length === 0) return;
		try {
			const response = await api.trades(codes.join(','));
			setTrades(response);
		} catch {
			setTrades({});
		}
	}, []);

	const handleBuy = (result: OvernightCandidate) => {
		const currentTrade = trades[result.code];
		if (currentTrade && currentTrade.action === 'buy') {
			messageApi.warning('已持有该股票');
			return;
		}

		if (result.price <= 0) {
			messageApi.error('无法获取当前价格');
			return;
		}

		setCurrentTradeAction('buy');
		setCurrentTradeStock(result);
		setTradeReason('');
		setShowTradeModal(true);
	};

	const handleSell = (result: OvernightCandidate) => {
		const currentTrade = trades[result.code];
		if (!currentTrade || currentTrade.action !== 'buy') {
			messageApi.warning('未持有该股票');
			return;
		}

		if (result.price <= 0) {
			messageApi.error('无法获取当前价格');
			return;
		}

		setCurrentTradeAction('sell');
		setCurrentTradeStock(result);
		setTradeReason('');
		setShowTradeModal(true);
	};

	const confirmTrade = async () => {
		if (!currentTradeStock) return;

		const price = currentTradeStock.price;
		setTradingLoading(true);
		try {
			await api.tradeCreate({
				code: currentTradeStock.code,
				name: currentTradeStock.name || '',
				action: currentTradeAction,
				price,
				signal: '隔夜套利',
				ktype: 'day',
				reason: tradeReason,
			});

			if (currentTradeAction === 'buy') {
				messageApi.success(`买入成功 @ ${price.toFixed(2)}`);
			} else {
				const currentTrade = trades[currentTradeStock.code];
				const profit = ((price - currentTrade.price) / currentTrade.price * 100).toFixed(2);
				const profitText = parseFloat(profit) >= 0 ? `+${profit}%` : `${profit}%`;
				messageApi.success(`卖出成功 @ ${price.toFixed(2)} (${profitText})`);
			}
			void loadTrades([currentTradeStock.code]);
			setShowTradeModal(false);
		} catch {
			messageApi.error(`${currentTradeAction === 'buy' ? '买入' : '卖出'}失败`);
		} finally {
			setTradingLoading(false);
		}
	};

	useEffect(() => {
		api.watchlist()
			.then((items) => {
				if (items.length === 0) return;
				setStockList((previous) => {
					const merged = [...previous];
					for (const item of items) {
						if (!merged.some((stock) => stock.code === item.code)) {
							merged.push({ code: item.code, name: item.name });
						}
					}
					return merged;
				});
			})
			.catch(() => {});
	}, []);

	const preloadCodesCache = useCallback(async (): Promise<Record<string, CodesCacheEntry>> => {
		const exchanges = ['sz', 'sh', 'bj'] as const;
		const merged: Record<string, CodesCacheEntry> = { ...codesCache };
		await Promise.all(
			exchanges.map(async (exchange) => {
				if (!merged[exchange] || Date.now() - merged[exchange].timestamp >= CACHE_EXPIRY) {
					try {
						const codesList = await api.codes(exchange);
						merged[exchange] = { list: codesList, timestamp: Date.now() };
					} catch {
						return;
					}
				}
			}),
		);
		setCodesCache(merged);
		return merged;
	}, [codesCache]);

	const loadBlocks = useCallback(async (file: string, typeFilter?: string) => {
		setBlockLoading(true);
		try {
			const response = await api.blockList(file, typeFilter || undefined, true);
			setBlockData(response.blocks || []);
			setSelectedBlock(null);
		} catch {
			setBlockData([]);
		} finally {
			setBlockLoading(false);
		}
	}, []);

	const loadBlockStocks = useCallback(async (block: BlockListItem) => {
		setBlockStocksLoading(true);
		try {
			const response = await api.blockShow(block.name, undefined, blockFile);
			if (response.stocks && response.stocks.length > 0) {
				const stocksWithNames = response.stocks.map((stock) => ({
					code: stock.code,
					name: stock.name?.trim() ? stock.name : stock.code,
				}));
				setSelectedBlock({
					name: block.name,
					type: block.type,
					count: block.count,
					stocks: response.stocks.map((stock) => stock.code),
					stocksWithNames,
				});
			} else {
				setSelectedBlock({ name: block.name, type: block.type, count: block.count });
			}
		} catch {
			setSelectedBlock({ name: block.name, type: block.type, count: block.count });
		} finally {
			setBlockStocksLoading(false);
		}
	}, [blockFile]);

	const handleSelectBlock = useCallback((block: BlockInfo) => {
		if (selectedBlock?.name === block.name) {
			setSelectedBlock(null);
			return;
		}
		void loadBlockStocks(block);
	}, [loadBlockStocks, selectedBlock]);

	useEffect(() => {
		if (sourceTab === 'block') {
			void loadBlocks(blockFile, ['block_zs.dat', 'block_fg.dat', 'block_gn.dat'].find((item) => item === blockFile) ? '' : undefined);
		}
	}, [sourceTab, blockFile, loadBlocks]);

	const resolvedCodes = useMemo(() => {
		if (sourceTab === 'block' && selectedBlock?.stocks) {
			return selectedBlock.stocks.join(',');
		}
		return stockList.map((stock) => stock.code).join(',');
	}, [selectedBlock, sourceTab, stockList]);

	const doScreen = async () => {
		const codes = resolvedCodes.trim();
		if (!codes) {
			messageApi.warning('请先选择股票来源');
			return;
		}

		setLoading(true);
		setError('');
		try {
			const codeArray = codes.split(',');
			const response = await api.overnightArbitrage(codeArray);
			setResults(response.final_candidates);
			setTotal(response.total);
			setFailedCodes(response.failed ?? []);
			setHasScreenLoaded(true);
			setIsOvernightTime(response.is_overnight_time);
			setCurrentTime(response.current_time);

			if (response.final_candidates.length > 0) {
				void loadTrades(response.final_candidates.map((item) => item.code));
			}
		} catch (screenError: unknown) {
			messageApi.error(screenError instanceof Error ? screenError.message : '筛选失败');
		} finally {
			setLoading(false);
		}
	};

	const [error, setError] = useState('');

	const filteredResults = useMemo(() => {
		return results;
	}, [results]);

	const sortedResults = useMemo(() => {
		const list = [...filteredResults];
		const dir = sortAsc ? 1 : -1;
		list.sort((a, b) => {
			let va: number | string = 0;
			let vb: number | string = 0;
			switch (sortKey) {
				case 'code':
					va = a.code;
					vb = b.code;
					break;
				case 'name':
					va = a.name || '';
					vb = b.name || '';
					break;
				case 'price':
					va = a.price;
					vb = b.price;
					break;
				case 'change_pct':
					va = a.change_pct;
					vb = b.change_pct;
					break;
			}
			if (typeof va === 'string' && typeof vb === 'string') {
				return va.localeCompare(vb) * dir;
			}
			return ((va as number) - (vb as number)) * dir;
		});
		return list;
	}, [filteredResults, sortAsc, sortKey]);

	const filteredBlocks = useMemo(() => {
		const sorted = [...blockData].sort((a, b) => b.count - a.count);
		if (!blockSearch) return sorted;
		const query = blockSearch.toLowerCase();
		return sorted.filter((block) => block.name.toLowerCase().includes(query));
	}, [blockData, blockSearch]);

	const handleSortChange = (key: SortKey) => {
		if (sortKey === key) {
			setSortAsc((previous) => !previous);
			return;
		}
		setSortKey(key);
		setSortAsc(true);
	};

	const addCodesFromInput = async () => {
		const codes = inputCode
			.split(/[, \n]+/)
			.map((value) => value.trim().toUpperCase())
			.filter(Boolean);

		if (codes.length === 0) return;

		const invalidCodes = codes.filter((value) => !/^\d{6}$/.test(value));
		if (invalidCodes.length > 0) {
			messageApi.error(`无效的股票代码: ${invalidCodes.join(', ')}`);
			return;
		}

		const existingCodes = codes.filter((value) => stockList.some((stock) => stock.code === value));
		if (existingCodes.length > 0) {
			messageApi.warning(`股票已存在: ${existingCodes.join(', ')}`);
		}

		const newCodes = codes.filter((value) => !stockList.some((stock) => stock.code === value));
		if (newCodes.length === 0) {
			setInputCode('');
			return;
		}

		setInputLoading(true);
		try {
			const cache = await preloadCodesCache();
			const grouped: Record<string, string[]> = { sz: [], sh: [], bj: [] };
			for (const code of newCodes) {
				if (code.startsWith('6')) grouped.sh.push(code);
				else if (code.startsWith('8') || code.startsWith('9')) grouped.bj.push(code);
				else grouped.sz.push(code);
			}

			const results: { code: string; name: string }[] = [];
			for (const [exchange, codeList] of Object.entries(grouped)) {
				if (codeList.length === 0) continue;
				const cached = cache[exchange];
				if (!cached) continue;
				for (const code of codeList) {
					const stockInfo = cached.list.find((item) => item.Code === code);
					if (stockInfo?.Name) {
						results.push({ code, name: stockInfo.Name });
					}
				}
			}

			if (results.length === 0) {
				messageApi.error('股票代码不存在');
			} else {
				setStockList((previous) => [...previous, ...results]);
				results.forEach((stock) => api.watchlistAdd(stock.code, stock.name).catch(() => {}));
				messageApi.success(results.length === 1 ? `已添加 ${results[0].name}` : `已添加 ${results.length} 只股票`);
			}
		} catch {
			messageApi.error('获取股票信息失败');
		} finally {
			setInputLoading(false);
			setInputCode('');
		}
	};

	const openBlockModal = async () => {
		if (!selectedBlock?.stocks?.length) return;
		setShowBlockModal(true);

		if (selectedBlock.stocksWithNames?.length) {
			setBlockStocksWithNames(selectedBlock.stocksWithNames);
			return;
		}

		setBlockStocksLoadingNames(true);
		try {
			const cache = await preloadCodesCache();
			const grouped: Record<string, string[]> = { sz: [], sh: [], bj: [] };
			for (const code of selectedBlock.stocks) {
				if (code.startsWith('6')) grouped.sh.push(code);
				else if (code.startsWith('8') || code.startsWith('9')) grouped.bj.push(code);
				else grouped.sz.push(code);
			}

			const rows: { code: string; name: string }[] = [];
			for (const [exchange, codeList] of Object.entries(grouped)) {
				if (codeList.length === 0) continue;
				const cached = cache[exchange];
				if (!cached) continue;
				for (const code of codeList) {
					const stockInfo = cached.list.find((item) => item.Code === code);
					if (stockInfo?.Name) {
						rows.push({ code, name: stockInfo.Name });
					} else {
						rows.push({ code, name: code });
					}
				}
			}

			const byCode = new Map(rows.map((row) => [row.code, row.name]));
			const filled = selectedBlock.stocks.map((code) => ({
				code,
				name: byCode.get(code) ?? code,
			}));
			setBlockStocksWithNames(filled);
		} finally {
			setBlockStocksLoadingNames(false);
		}
	};

	const addAllBlockStocksToWatchlist = () => {
		const newStocks = blockStocksWithNames
			.filter((stock) => !stockList.some((watch) => watch.code === stock.code))
			.map((stock) => ({ code: stock.code, name: stock.name }));

		if (newStocks.length === 0) {
			messageApi.warning('所有股票已存在');
			return;
		}

		setStockList((previous) => [...previous, ...newStocks]);
		newStocks.forEach((stock) => api.watchlistAdd(stock.code, stock.name).catch(() => {}));
		messageApi.success(`已添加 ${newStocks.length} 只股票`);
	};

	return (
		<>
			{contextHolder}
			<Space direction="vertical" size={16} style={{ width: '100%' }}>
				<Flex justify="space-between" align="center" wrap="wrap" gap={12}>
					<div>
						<Title level={3} style={{ margin: 0 }}>杨永兴隔夜套利法</Title>
						<Paragraph type="secondary" style={{ marginBottom: 0 }}>
							尾盘14:30后选股，次日早盘卖出。筛选标准：涨幅3%-5%、量比&gt;1、换手率5%-10%、流通市值50-200亿、近20日涨停、MA多头排列、股价站在均价线上方。
						</Paragraph>
					</div>
				</Flex>

				{!isOvernightTime && (
					<Alert
						type="warning"
						showIcon
						message="当前时间不是最佳筛选时间"
						description="杨永兴策略建议在14:30之后进行选股，此时全天走势基本定型，能更好判断主力意图。"
					/>
				)}

				<Card size="small" hoverable style={{ cursor: 'pointer' }} onClick={() => setShowSourceModal(true)}>
					<Flex justify="space-between" align="center">
						<Space>
							<Text type="secondary">股票池：</Text>
							{sourceTab === 'watchlist' ? (
								<Text strong>{stockList.length} 只自选股</Text>
							) : selectedBlock ? (
								<Text strong>{selectedBlock.name}（{selectedBlock.stocks?.length || selectedBlock.count} 只）</Text>
							) : (
								<Text type="secondary">未选择板块</Text>
							)}
						</Space>
						<Button size="small" icon={<EditOutlined />}>更换</Button>
					</Flex>
				</Card>

				<Card title="选股标准说明" size="small">
					<Space direction="vertical" size={12} style={{ width: '100%' }}>
						<Flex gap={16} wrap>
							<div style={{ display: 'flex', gap: 8 }}>
								<Tag color="green" style={{ width: 20, textAlign: 'center' }}>✓</Tag>
								<Text>涨幅3%-5%：既有资金运作，又未过度透支</Text>
							</div>
							<div style={{ display: 'flex', gap: 8 }}>
								<Tag color="green" style={{ width: 20, textAlign: 'center' }}>✓</Tag>
								<Text>量比&gt;1：新增资金持续进场</Text>
							</div>
							<div style={{ display: 'flex', gap: 8 }}>
								<Tag color="green" style={{ width: 20, textAlign: 'center' }}>✓</Tag>
								<Text>换手率5%-10%：交投活跃但不疯狂出货</Text>
							</div>
							<div style={{ display: 'flex', gap: 8 }}>
								<Tag color="green" style={{ width: 20, textAlign: 'center' }}>✓</Tag>
								<Text>流通市值50-200亿：中小盘，弹性充足</Text>
							</div>
							<div style={{ display: 'flex', gap: 8 }}>
								<Tag color="green" style={{ width: 20, textAlign: 'center' }}>✓</Tag>
								<Text>近20日涨停：有短线资金关注</Text>
							</div>
							<div style={{ display: 'flex', gap: 8 }}>
								<Tag color="green" style={{ width: 20, textAlign: 'center' }}>✓</Tag>
								<Text>MA多头排列：MA5&gt;MA10&gt;MA20</Text>
							</div>
							<div style={{ display: 'flex', gap: 8 }}>
								<Tag color="green" style={{ width: 20, textAlign: 'center' }}>✓</Tag>
								<Text>股价站在均价线上方：多头主导</Text>
							</div>
						</Flex>
						<Alert
							type="info"
							showIcon
							message="卖出铁律"
							description="次日早盘10:00前必须全部清仓，无论盈亏，除非一字/缩量涨停可留。追求1%-3%溢价即可兑现。"
						/>
					</Space>
				</Card>

				<Button
					type="primary"
					icon={<SearchOutlined />}
					loading={loading}
					onClick={() => void doScreen()}
					disabled={!resolvedCodes.trim()}
					size="large"
				>
					开始筛选（{stockList.length}只股票）
				</Button>

				{error && <Alert type="error" showIcon message="筛选失败" description={error} />}

				{hasScreenLoaded && (
					<Card size="small" style={{ background: 'linear-gradient(135deg, rgba(22,119,255,0.08), rgba(14,165,233,0.06))' }}>
						<Flex justify="space-between" align="center" wrap="wrap" gap={16}>
							<Space size={24}>
								<Statistic title="扫描总数" value={total} suffix="只" style={{ fontSize: 13 }} />
								<Statistic title="最终候选" value={results.length} suffix="只" style={{ fontSize: 13 }} />
							</Space>
							<Space size={[6, 6]} wrap>
								<Tag color="blue">当前时间: {currentTime}</Tag>
								<Tag color={isOvernightTime ? 'green' : 'orange'}>{isOvernightTime ? '✓ 最佳筛选时间' : '建议14:30后筛选'}</Tag>
							</Space>
						</Flex>
					</Card>
				)}

				{hasScreenLoaded && failedCodes.length > 0 && (
					<Collapse
						items={[{
							key: 'failed',
							label: <Space><Tag color="error">失败/淘汰 {failedCodes.length}</Tag><Text type="secondary">点击查看详情</Text></Space>,
							children: (
								<Space direction="vertical" size={8} style={{ width: '100%' }}>
									<List
										size="small"
										dataSource={failedCodes.slice(0, 50)}
										renderItem={(item) => (
											<List.Item>
												<Flex justify="space-between" align="center" style={{ width: '100%' }}>
													<Space>
														<Text code>{item.code}</Text>
													</Space>
													<Text type="danger" style={{ fontSize: 12 }}>{item.reason}</Text>
												</Flex>
											</List.Item>
										)}
									/>
									{failedCodes.length > 50 && (
										<Text type="secondary" style={{ fontSize: 12, textAlign: 'center', display: 'block' }}>
											仅显示前50条，共{failedCodes.length}条
										</Text>
									)}
								</Space>
							),
						}]}
					/>
				)}

				{sortedResults.length > 0 ? (
					<VirtualResultTable
						results={sortedResults}
						tableContainerRef={tableContainerRef}
						sortKey={sortKey}
						sortAsc={sortAsc}
						onSortChange={handleSortChange}
						navigate={navigate}
						trades={trades}
						tradingLoading={tradingLoading}
						handleBuy={handleBuy}
						handleSell={handleSell}
					/>
				) : !loading && !error ? (
					<Card>
						<Empty
							image={Empty.PRESENTED_IMAGE_SIMPLE}
							description={hasScreenLoaded ? '当前筛选条件下没有命中结果' : '选择股票来源后点击"开始筛选"'}
						/>
					</Card>
				) : null}
			</Space>

			<Modal
				open={showBlockModal}
				onCancel={() => setShowBlockModal(false)}
				footer={[
					<Button key="close" onClick={() => setShowBlockModal(false)}>关闭</Button>,
					<Button key="add-all" type="primary" icon={<PlusOutlined />} onClick={addAllBlockStocksToWatchlist}>
						全部加入自选
					</Button>,
				]}
				width={760}
				title={selectedBlock ? `${selectedBlock.name} 成分股` : '成分股'}
			>
				{blockStocksLoadingNames ? (
					<Flex justify="center" align="center" style={{ minHeight: 240 }}><Spin /></Flex>
				) : (
					<List
						grid={{ gutter: 12, column: 2 }}
						dataSource={blockStocksWithNames}
						renderItem={(stock) => {
							const inWatchlist = stockList.some((item) => item.code === stock.code);
							return (
								<List.Item>
									<Card size="small" hoverable onClick={() => {
										setShowBlockModal(false);
										navigate(`/stock/${stock.code}/chart`);
									}}>
										<Flex justify="space-between" align="center" gap={12}>
											<Space direction="vertical" size={2}>
												<Text code>{stock.code}</Text>
												<Text>{stock.name}</Text>
											</Space>
											<Button
												size="small"
												type={inWatchlist ? 'default' : 'primary'}
												icon={inWatchlist ? <CloseOutlined /> : <PlusOutlined />}
												onClick={(event) => {
													event.stopPropagation();
													if (inWatchlist) {
														api.watchlistDelete(stock.code).catch(() => {});
														setStockList((previous) => previous.filter((item) => item.code !== stock.code));
														messageApi.success(`已移除 ${stock.name}`);
													} else {
														api.watchlistAdd(stock.code, stock.name).catch(() => {});
														setStockList((previous) => [...previous, { code: stock.code, name: stock.name }]);
														messageApi.success(`已添加 ${stock.name}`);
													}
												}}
											>
												{inWatchlist ? '移除' : '加入自选'}
											</Button>
										</Flex>
									</Card>
								</List.Item>
							);
						}}
					/>
				)}
			</Modal>

			<Modal
				title="选择股票来源"
				open={showSourceModal}
				onCancel={() => setShowSourceModal(false)}
				footer={[
					<Button key="cancel" onClick={() => setShowSourceModal(false)}>关闭</Button>,
					<Button key="confirm" type="primary" onClick={() => setShowSourceModal(false)}>确定</Button>,
				]}
				width={760}
				style={{ maxHeight: '70vh' }}
			>
				<Segmented<SourceTab>
					value={sourceTab}
					onChange={(value) => setSourceTab(value)}
					options={[
						{ label: '自选股', value: 'watchlist' },
						{ label: '板块', value: 'block' },
					]}
					style={{ marginBottom: 16 }}
				/>

				{sourceTab === 'watchlist' ? (
					<Space direction="vertical" size={12} style={{ width: '100%' }}>
						<Input
							prefix={<SearchOutlined />}
							value={inputCode}
							onChange={(event) => setInputCode(event.target.value)}
							onPressEnter={() => void addCodesFromInput()}
							placeholder="输入股票代码，支持逗号/空格分隔"
							suffix={inputLoading ? <Spin size="small" /> : null}
						/>
						<Flex justify="space-between" align="center" gap={8}>
							<Text type="secondary">共 {stockList.length} 只股票</Text>
						</Flex>
						<div style={{ maxHeight: 280, overflow: 'auto' }}>
							{stockList.length === 0 ? (
								<Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="输入股票代码后回车添加" />
							) : (
								<List
									size="small"
									dataSource={stockList}
									renderItem={(stock, index) => (
										<List.Item
											style={{ cursor: 'pointer' }}
											onClick={() => {
												setShowSourceModal(false);
												navigate(`/stock/${stock.code}/chart`);
											}}
											actions={[
												<Button
													key="remove"
													type="text"
													danger
													icon={<CloseOutlined />}
													onClick={(event) => {
														event.stopPropagation();
														api.watchlistDelete(stock.code).catch(() => {});
														setStockList((previous) => previous.filter((_, itemIndex) => itemIndex !== index));
													}}
												/>,
											]}
										>
											<List.Item.Meta
												title={<Space><Text code>{stock.code}</Text><Text>{stock.name || '-'}</Text></Space>}
											/>
										</List.Item>
									)}
								/>
							)}
						</div>
					</Space>
				) : (
					<Space direction="vertical" size={12} style={{ width: '100%' }}>
						<Segmented
							block
							value={blockFile}
							onChange={(value) => {
								const nextFile = String(value);
								setBlockFile(nextFile);
								void loadBlocks(nextFile, undefined);
							}}
							options={[
								{ label: '综合板块', value: 'block_zs.dat' },
								{ label: '概念板块', value: 'block_gn.dat' },
								{ label: '风格板块', value: 'block_fg.dat' },
							]}
						/>
						<Input
							prefix={<SearchOutlined />}
							value={blockSearch}
							onChange={(event) => setBlockSearch(event.target.value)}
							placeholder="搜索板块..."
						/>
						<div style={{ maxHeight: 280, overflow: 'auto' }}>
							{blockLoading ? (
								<Flex justify="center" align="center" style={{ minHeight: 240 }}><Spin /></Flex>
							) : (
								<List
									size="small"
									dataSource={filteredBlocks}
									renderItem={(block) => (
										<List.Item
											style={{
												cursor: 'pointer',
												borderRadius: 8,
												paddingInline: 12,
												background: selectedBlock?.name === block.name ? 'var(--ant-color-primary-bg)' : undefined,
											}}
											onClick={() => handleSelectBlock(block)}
										>
											<Flex justify="space-between" align="center" style={{ width: '100%' }}>
												<Text ellipsis style={{ maxWidth: 180 }}>{block.name}</Text>
												<Tag>{block.count}只</Tag>
											</Flex>
										</List.Item>
									)}
								/>
							)}
						</div>
						{selectedBlock && (
							<Alert
								type="info"
								showIcon
								message={`已选 ${selectedBlock.name}`}
								description={
									<Space wrap>
										<Text>{blockStocksLoading ? '加载成分股中...' : `${selectedBlock.stocks?.length || selectedBlock.count} 只股票`}</Text>
										<Button size="small" icon={<InfoCircleOutlined />} onClick={() => void openBlockModal()} disabled={!selectedBlock.stocks?.length}>
											查看成分股
										</Button>
									</Space>
								}
							/>
						)}
					</Space>
				)}
			</Modal>

			<Modal
				title={currentTradeAction === 'buy' ? '买入确认' : '卖出确认'}
				open={showTradeModal}
				onCancel={() => setShowTradeModal(false)}
				footer={[
					<Button key="cancel" onClick={() => setShowTradeModal(false)}>取消</Button>,
					<Button key="confirm" type={currentTradeAction === 'buy' ? 'primary' : undefined} danger={currentTradeAction === 'sell'} onClick={confirmTrade} loading={tradingLoading}>
						{currentTradeAction === 'buy' ? '确认买入' : '确认卖出'}
					</Button>,
				]}
				width={520}
			>
				{currentTradeStock && (
					<Space direction="vertical" size={16} style={{ width: '100%' }}>
						<Card size="small">
							<Flex justify="space-between" align="center">
								<Space direction="vertical">
									<Text code style={{ fontSize: 16 }}>{currentTradeStock.code}</Text>
									<Text style={{ fontSize: 16 }}>{currentTradeStock.name}</Text>
								</Space>
								<Text style={{ fontSize: 20, color: getPriceColor(currentTradeStock.change_pct) }}>
									{currentTradeStock.price.toFixed(2)}
								</Text>
							</Flex>
						</Card>

						<div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 12 }}>
							<div>
								<Text type="secondary" style={{ fontSize: 12 }}>涨幅</Text>
								<div style={{ fontSize: 14, color: getPriceColor(currentTradeStock.change_pct) }}>
									{formatPercent(currentTradeStock.change_pct)}
								</div>
							</div>
							<div>
								<Text type="secondary" style={{ fontSize: 12 }}>量比</Text>
								<div style={{ fontSize: 14 }}>{currentTradeStock.volume_ratio.toFixed(2)}</div>
							</div>
							<div>
								<Text type="secondary" style={{ fontSize: 12 }}>换手率</Text>
								<div style={{ fontSize: 14 }}>{currentTradeStock.turnover_rate.toFixed(2)}%</div>
							</div>
							<div>
								<Text type="secondary" style={{ fontSize: 12 }}>流通市值</Text>
								<div style={{ fontSize: 14 }}>{currentTradeStock.market_cap.toFixed(2)}亿</div>
							</div>
						</div>

						<Input.TextArea
							value={tradeReason}
							onChange={(event) => setTradeReason(event.target.value)}
							placeholder="输入交易备注（可选）"
							rows={3}
						/>

						<Alert
							type="warning"
							showIcon
							description={currentTradeAction === 'buy' ? '根据杨永兴策略，次日早盘10:00前建议卖出' : '确认卖出后将清空持仓'}
						/>
					</Space>
				)}
			</Modal>
		</>
	);
}
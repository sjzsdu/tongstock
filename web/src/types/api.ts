export interface KlineItem {
  Time: string;
  Open: number;
  High: number;
  Low: number;
  Close: number;
  Volume: number;
  Amount: number;
}

export interface Quote {
  Code: string;
  Name: string;
  Price: number;
  Open: number;
  High: number;
  Low: number;
  LastClose: number;
  Volume: number;
  Amount: number;
  SVol: number;
  BVol: number;
}

export interface MACDResult {
  DIF: number[];
  DEA: number[];
  Hist: number[];
  HIST?: number[];
}

export interface KDJResult {
  K: number[];
  D: number[];
  J: number[];
}

export interface BOLLResult {
  Upper: number[];
  Middle: number[];
  Lower: number[];
}

export interface IndicatorData {
  code: string;
  type: string;
  category: string;
  count: number;
  last: KlineItem;
  klines: KlineItem[];
  ma: Record<string, number[]>;
  macd: MACDResult | null;
  kdj: KDJResult | null;
  boll: BOLLResult | null;
  rsi: Record<string, number[]>;
  signals: Signal[];
}

export interface Signal {
  Code: string;
  Date: string;
  Type: string;
  Indicator: string;
  Details: string;
  Strength: number;
}

export interface Finance {
  ZongGuBen: number;
  LiuTongGuBen: number;
  ZongZiChan: number;
  JingZiChan: number;
  ZhuYingShouRu: number;
  JingLiRun: number;
  MeiGuJingZiChan: number;
  GuDongRenShu: number;
  IPODate: number;
  UpdatedDate: number;
}

export interface FinanceTrendRecord {
  period: string;
  year: number;
  quarter: string;
  label: string;
  revenue?: number;
  netProfit?: number;
  grossMargin?: number;
  netMargin?: number;
  roe?: number;
  eps?: number;
  operatingCashPerShare?: number;
}

export interface FinanceTrendsResponse {
  code: string;
  mode: 'quarter' | 'year' | string;
  metrics: string[];
  records: FinanceTrendRecord[];
  available: string[];
}

export interface FinanceMetricRow {
  name: string;
  values: string[];
}

export interface FinanceMetricTable {
  title: string;
  periods: string[];
  rows: FinanceMetricRow[];
}

export interface FinanceMetricsResponse {
  code: string;
  tables: FinanceMetricTable[];
}

export interface XdXrItem {
  Date: string;
  Category: string;
  FenHong: number;
  PeiGuJia: number;
  SongZhuanGu: number;
  PeiGu: number;
  PanHouLiuTong: number;
  HouZongGuBen: number;
}

export interface CompanyCategory {
  Filename: string;
  Name: string;
  Start: number;
  Length: number;
}

export interface MinuteItem {
  Time: string;
  Price: number;
  Number: number;
}

export interface TradeItem {
  Time: string;
  Price: number;
  Volume: number;
  Status: number;
}

export interface AuctionItem {
  time: string;
  price: number;
  match: number;
  unmatched: number;
  flag: number;
}

export interface BlockItem {
  BlockName: string;
  StockCode: string;
  BlockType: number;
}

export interface BlockListItem {
  name: string;
  type: number;
  count: number;
  stocks?: string[];
}

export interface BlockListResponse {
  blocks: BlockListItem[];
  file: string;
  total: number;
}

export interface CodeItem {
  Code: string;
  Name: string;
}

export interface SearchStockMatch {
  code: string;
  name: string;
  exchange: string;
  matchType: string;
}

export interface StockSearchIndexItem {
  code: string;
  name: string;
  exchange: string;
  nameNorm: string;
  pinyin: string;
  initials: string;
}

export interface StockSearchIndexResponse {
  updatedAt: number;
  total: number;
  items: StockSearchIndexItem[];
}

export interface StockSearchResponse {
  query: string;
  total: number;
  exact: boolean;
  resolved: boolean;
  matches: SearchStockMatch[];
}

export interface IndexBar extends KlineItem {
  UpCount: number;
  DownCount: number;
}

export interface ScreenResult {
  code: string;
  name: string;
  last: KlineItem;
  ma: Record<string, number[]>;
  macd: MACDResult | null;
  kdj: KDJResult | null;
  signals: Signal[];
}

export interface ScreenCodeStatus {
  code: string;
  name?: string;
  status: 'failed' | 'skipped';
  reason: string;
}

export interface ScreenResponse {
  results: ScreenResult[];
  total: number;
  matched?: number;
  successCount?: number;
  failedCount?: number;
  skippedCount?: number;
  failed?: ScreenCodeStatus[];
  skipped?: ScreenCodeStatus[];
  capped?: boolean;
  maxCodes?: number;
  reason?: string;
}

export interface SignalOutcome {
  date: string;
  type: string;
  indicator: string;
  details: string;
  price: number;
  chg1: number | null;
  chg5: number | null;
  chg10: number | null;
  chg20: number | null;
  action: string;
}

export interface SignalSummary {
  type: string;
  action: string;
  count: number;
  valid1: number;
  valid5: number;
  valid10: number;
  valid20: number;
  win1: number;
  win5: number;
  win10: number;
  win20: number;
  avg1: number;
  avg5: number;
  avg10: number;
  avg20: number;
}

export interface SignalInterpretation {
  summary: string;
  explanation: string;
  suggestions: string[];
  risk_level: string;
  trend: string;
}

export interface SignalWithInterpretation {
  signal: {
    type: string;
    indicator: string;
    date: string;
    strength: number;
    details: string;
  };
  interpretation: SignalInterpretation;
}

export interface SignalAnalysis {
  code: string;
  type: string;
  count: number;
  signals: number;
  overall_summary: string;
  trend: string;
  interpretations: SignalWithInterpretation[];
  outcomes: SignalOutcome[];
  summary: SignalSummary[];
}

export interface HistoryStock {
  code: string;
  name?: string;
  analyzed_at: string;
}

export interface WatchlistStock {
  code: string;
  name?: string;
  group?: string;
  note?: string;
  added_at: string;
  updated_at?: string;
}

export interface KlineSyncState {
  code: string;
  ktype: number;
  first_date?: string;
  last_date?: string;
  row_count: number;
  last_sync_at: string;
  status: string;
  error?: string;
}

export interface KlineSyncResult {
  code: string;
  mode: string;
  status: string;
  count: number;
  state?: KlineSyncState;
  error?: string;
}

export interface KlineBatchSyncResult {
  total: number;
  success: number;
  failed: number;
  results: KlineSyncResult[];
}

export interface IndicatorParams {
  ma: number[];
  macd: { fast: number; slow: number; signal: number };
  kdj: { n: number; m1: number; m2: number };
  boll: { n: number; k: number };
  rsi: number[];
}

export interface IndicatorConfig {
  defaults: IndicatorParams;
  categories: Record<string, Partial<IndicatorParams>>;
  overrides: Record<string, Partial<IndicatorParams>>;
  path?: string;
}

export interface BlockComparisonStock {
  code: string;
  name: string;
  price: number;
  change: number;
}

export interface BlockComparison {
  block_name: string;
  block_type: number;
  block_file: string;
  total_stocks: number;
  valid_stocks: number;
  up_count: number;
  down_count: number;
  avg_change: number;
  stock_rank: number;
  stock_change: number;
  capped?: boolean;
  stock_quote: {
    code: string;
    name: string;
    price: number;
    change: number;
    last_close: number;
  };
  top_stocks: BlockComparisonStock[];
  bottom_stocks: BlockComparisonStock[];
}

export interface StockCompareResponse {
  code: string;
  stock_name: string;
  stock_change: number;
  comparisons: BlockComparison[];
}

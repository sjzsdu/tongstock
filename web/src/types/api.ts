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

export type QuoteItem = Quote;

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

export interface SyncFreshnessResult {
  code: string;
  status: string;
  last_date?: string;
  last_sync_at?: string;
  row_count?: number;
  freshness: 'fresh' | 'stale' | 'outdated' | 'failed' | 'empty' | 'unknown';
  stale_reason?: string;
  error?: string;
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

// Agent types
export interface AgentState {
  started_at: string;
  workspace: string;
  defaults: {
    agent: string;
    model: string;
    session: string;
    debug: boolean;
    stock_agent?: string;
  };
  agents: AgentInfo[];
}

export interface AgentDiagnosticResponse {
  enabled: boolean;
  ready: boolean;
  checks: string[];
  errors?: string[];
  hints?: string[];
}

export interface AgentInfo {
  id: string;
  name: string;
  description?: string;
  skills?: string[];
  tools?: string[];
}

export interface AgentChatResponse {
  response?: string;
  error?: string;
}

export interface AgentSessionsResponse {
  sessions: AgentSessionInfo[];
  missing?: boolean;
  message?: string;
}

export interface AgentSessionInfo {
  session: string;
  agent?: string;
  path: string;
  updated_at?: string;
  size?: number;
}

export interface AgentTranscriptResponse {
  session: string;
  agent?: string;
  path?: string;
  messages?: { role: string; content: string }[];
  missing?: boolean;
  message?: string;
}

export interface AgentDebateResponse {
  stock_code: string;
  stock_name?: string;
  topic: string;
  participants: {
    agent: string;
    agent_name: string;
    response: string;
    error?: string;
  }[];
  summary?: string;
  error?: string;
}

// Custom stock pool types
export type FilterField = 'marketCap' | 'price' | 'turnoverRate' | 'changePct' | 'volumeRatio' | 'exchange' | 'board' | 'excludeST';
export type FilterOperator = 'between' | 'gt' | 'gte' | 'lt' | 'lte' | 'eq' | 'in';

export interface StockPoolFilter {
  field: FilterField;
  operator: FilterOperator;
  value: (number | string | boolean)[];
  label?: string;
}

export interface CustomStockPool {
  id: string;
  name: string;
  description?: string;
  filters: StockPoolFilter[];
  createdAt: string;
  updatedAt: string;
}

export interface MarketCodeItem {
  code: string;
  name: string;
  exchange: string;
}

export interface MarketCodesResponse {
  total: number;
  codes: MarketCodeItem[];
}

// Paradigm types
export interface ParadigmItem {
  id: string;
  name: string;
  side: 'buy' | 'sell';
  agent_text?: string;
  context: {
    market_cap: string;
    shareholder_dominant: string;
    activity?: string;
    trend?: string;
  };
  stock_code?: string;
  stock_name?: string;
  buy_conditions: { indicator: string; operator: string; value: string }[];
  sell_conditions: {
    take_profit?: { indicator: string; operator: string; value: string }[];
    stop_loss?: { indicator: string; operator: string; value: string }[];
  };
  confirmations?: string[];
  invalidations?: string[];
  expectation: {
    holding_period: string;
    expected_return: string;
    risk_reward_ratio: string;
    win_rate?: number;
    sample_size?: number;
    confidence: number;
  };
  rationale?: string;
  source?: {
    agent_version?: string;
    model?: string;
    kline_type?: string;
    days?: number;
    generated_at?: string;
    data_window?: string;
    cache_key?: string;
  };
  validation?: {
    valid: boolean;
    errors?: string[];
    warnings?: string[];
    auto_evaluable: number;
    total_conditions: number;
    auto_evaluable_ratio: number;
    data_completeness: number;
    reliability_label?: string;
  };
  review_status?: string;
  review_note?: string;
  review_rating?: number;
  actual_return?: number;
  created_at: string;
  updated_at: string;
}

export interface EvaluatedItem {
  text: string;
  status: 'met' | 'not_met' | 'unknown';
  reason?: string;
}

export interface ParadigmAnalyzeResponse {
  stock_code: string;
  stock_name?: string;
  paradigm?: ParadigmItem;
  evaluated_confirm?: EvaluatedItem[];
  evaluated_invalid?: EvaluatedItem[];
  agent_text: string;
  cached?: boolean;
  message?: string;
  error?: string;
}

export interface ParadigmListResponse {
  paradigms: ParadigmItem[];
  total: number;
}

export interface EvaluatedCondition {
  condition: string;
  type: 'buy' | 'take_profit' | 'stop_loss';
  status: 'met' | 'not_met' | 'unknown';
  value?: string;
}

export interface ParadigmEvaluateResponse {
  stock_code: string;
  conditions: EvaluatedCondition[];
  error?: string;
}

export interface ParadigmAlertItem {
  paradigm_id: string;
  stock_code: string;
  stock_name?: string;
  side: string;
  type: string;
  condition: string;
  status: string;
  value?: string;
  severity: 'info' | 'warning' | 'critical' | string;
}

export interface ParadigmAlertsResponse {
  alerts: ParadigmAlertItem[];
  total: number;
}

export interface ParadigmStatsResponse {
  total: number;
  reviewed: number;
  verified: number;
  rejected: number;
  win_rate: number;
  average_return: number;
  average_rating: number;
  high_reliability: number;
}

export interface ParadigmBacktestItem {
  paradigm_id: string;
  stock_code: string;
  sample_size: number;
  win_rate_5: number;
  win_rate_10: number;
  win_rate_20: number;
  avg_return_5: number;
  avg_return_10: number;
  avg_return_20: number;
  max_drawdown: number;
  error?: string;
}

export interface ChatSessionInfo {
  id: string;
  stock_code: string;
  stock_name?: string;
  agent?: string;
  messages: { role: string; content: string; timestamp?: string }[];
  created_at: string;
  updated_at: string;
}

// Newsfeed types
export interface NewsItem {
  id: string;
  source: string;
  newsType: string;
  title: string;
  summary: string;
  content: string;
  publishTime: string;
  hotScore: number;
  tags: string[];
  relatedStocks: string[];
  url: string;
  originalId: string;
  createdAt: string;
  updatedAt: string;
}

export interface NewsSummary {
  id: string;
  source: string;
  newsType: string;
  title: string;
  summary: string;
  publishTime: string;
  hotScore: number;
  tags: string[];
  relatedStocks: string[];
  url?: string;
}

export interface FeedResult {
  total: number;
  items: NewsSummary[];
}

export interface HotEvent {
  id: string;
  title: string;
  keywords: string[];
  relatedStocks: string[];
  hotIndex: number;
  sourceCounts: Record<string, number>;
  newsItemIDs: string[];
  status: string;
  createdAt: string;
  updatedAt: string;
}

export interface EventResult {
  total: number;
  items: HotEvent[];
}

export interface SentimentResult {
  type: string;
  score: number;
  confidence: number;
}

export interface MarketSentiment {
  timestamp: string;
  positiveCount: number;
  negativeCount: number;
  neutralCount: number;
  sentimentIndex: number;
  hotScoreAvg: number;
  sentimentByType: Record<string, SentimentResult>;
  sentimentBySource: Record<string, SentimentResult>;
}

export interface SentimentTrend {
  time: string;
  sentiment: number;
  newsCount: number;
}

export interface SentimentHeatmapItem {
  stockCode: string;
  stockName: string;
  sentiment: SentimentResult;
  newsCount: number;
  hotScore: number;
}

export interface AlertRecord {
  id: string;
  ruleID: string;
  newsID: string;
  stockCode: string;
  level: string;
  type: string;
  title: string;
  summary: string;
  source: string;
  read: boolean;
  triggerTime: string;
  createdAt: string;
}

export interface AlertRule {
  id: string;
  name: string;
  level: string;
  stockCodes: string[];
  minHotScore: number;
  sentiment: string;
  enabled: boolean;
  lastTrigger: string;
  createdAt: string;
}

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
  freshness?: 'fresh' | 'stale' | 'outdated' | 'failed' | 'empty' | 'unknown';
  stale_reason?: string;
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

export interface AgentResearchRequest {
  paradigm_id: string;
  question?: string;
  snapshot_id?: string;
  start_date?: string;
  end_date?: string;
}

export interface AgentResearchResponse {
  conclusion: 'evidence_passed' | 'not_verified' | string;
  answer: string;
  citation: {
    experiment_id: string;
    run_id: string;
    snapshot_id: string;
    evidence_hash: string;
    result_hash: string;
    trade_ids?: string[];
  };
  evidence: EvidenceCard;
  critic: {
    id: string;
    target_id: string;
    conclusion: string;
    hard_blocked: boolean;
    summary?: string;
    issues: Array<{
      id: string;
      dimension: string;
      severity: string;
      title: string;
      evidence?: string;
      is_hard_threshold: boolean;
    }>;
  };
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
  experiment_id?: string;
  run_id?: string;
  evidence_hash?: string;
  research?: AgentResearchResponse;
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
  experiment_id: string;
  run_id: string;
  snapshot_id: string;
  config_hash: string;
  result_hash: string;
  metrics: {
    total_trades?: number;
    win_rate?: number;
    total_return?: number;
    max_drawdown?: number;
    net_pnl?: number;
    custom?: Record<string, number>;
  };
  segmented_metrics: unknown[];
  artifacts: Array<{
    id: string;
    type: string;
    name: string;
    content_hash?: string;
    content?: unknown;
  }>;
}

// Evidence Card types
export interface SampleResult {
  period: string;
  sample_size: number;
  total_return?: number;
  annual_return?: number;
  sharpe_ratio?: number;
  win_rate?: number;
  max_drawdown?: number;
  trades_count: number;
  gross_pnl?: number;
  net_pnl?: number;
}

export interface CIResult {
  period: string;
  sample_size: number;
  mean_return: number;
  ci_95_lower: number;
  ci_95_upper: number;
  ci_95_width: number;
  t_statistic: number;
  p_value: number;
  significant: boolean;
  notes?: string[];
}

export interface CostBreakdown {
  gross_return?: number;
  net_return?: number;
  total_cost: number;
  cost_per_trade?: number;
  cost_ratio?: number;
  net_retention?: number;
  slippage_cost: number;
  commission_cost: number;
  tax_cost: number;
  transfer_fee: number;
}

export interface DrawdownInfo {
  max_drawdown: number;
  drawdown_ratio?: number;
  warning?: string;
}

export interface ParamSweep {
  param_name: string;
  param_value: number;
  return: number;
  change_pct: number;
}

export interface ParamSensitivityInfo {
  sensitivity_index: number;
  perturbation_pass: boolean;
  perturbation_delta: number;
  nearby_params: ParamSweep[];
  warning?: string;
}

export interface HoldingItem {
  stock_code: string;
  stock_name: string;
  weight: number;
}

export interface ConcentrationInfo {
  max_position_weight: number;
  concentration_index: number;
  top_holdings: HoldingItem[];
  sector_exposure?: Record<string, number>;
  diversification_score: number;
}

export interface CounterExample {
  type: string;
  description: string;
  period: string;
  return?: number;
  reason: string;
  severity: string;
}

export interface RiskFlag {
  category: string;
  level: string;
  message: string;
  mitigation?: string;
}

export interface ReviewRecord {
  reviewer: string;
  action: string;
  note?: string;
  rating?: number;
  timestamp: string;
}

export interface DataLineage {
  data_source: string;
  data_version: string;
  data_range: string;
  data_start: string;
  data_end: string;
  last_updated: string;
  generated_by: string;
  generated_at: string;
  source_hash: string;
  snapshot_id: string;
  experiment_id: string;
  run_id: string;
  result_hash: string;
  artifact_hashes: Record<string, string>;
  kline_manifest_hashes: Record<string, string>;
  review_history?: ReviewRecord[];
}

export interface TradeRecord {
  trade_id: string;
  window: number;
  segment: string;
  stock_code: string;
  buy_signal_date: string;
  buy_execution_date: string;
  sell_signal_date: string;
  sell_execution_date: string;
  quantity: number;
  buy_price: number;
  sell_price: number;
  gross_pnl: number;
  net_pnl: number;
  total_cost: number;
  return?: number;
}

export interface EvidenceCard {
  paradigm_id: string;
  paradigm_name: string;
  stock_code: string;
  stock_name: string;
  generated_at: string;
  available: boolean;
  promotion_eligible: boolean;
  unavailable_reasons?: string[];
  promotion_blockers?: string[];
  experiment_id?: string;
  run_id?: string;
  snapshot_id?: string;
  evidence_hash?: string;
  result_hash?: string;
  in_sample?: SampleResult;
  out_of_sample?: SampleResult;
  confidence_interval?: CIResult;
  cost_analysis?: CostBreakdown;
  drawdown_analysis?: DrawdownInfo;
  param_sensitivity?: ParamSensitivityInfo;
  concentration?: ConcentrationInfo;
  counter_evidence: CounterExample[];
  risk_flags: RiskFlag[];
  lineage?: DataLineage;
  trade_samples?: TradeRecord[];
}

// Paradigm lifecycle state machine types
export type ParadigmState =
  | 'pending'
  | 'reviewed'
  | 'verified'
  | 'promoted'
  | 'degraded'
  | 'suspended'
  | 'rejected';

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

// Forward Run & Signal Ledger types
export interface ConstraintsSnapshot {
  enable_t_1: boolean;
  enable_price_limit: boolean;
  enable_suspension: boolean;
  board: string;
  min_trade_unit: number;
  commission_rate: number;
  slippage_bps: number;
  stamp_duty_rate: number;
  min_commission: number;
  transfer_fee_rate: number;
}

export interface ForwardPositionState {
  stock_code: string;
  quantity: number;
  average_price: number;
  accrued_buy_fees: number;
  buy_date: string;
  last_price: number;
  updated_at: string;
}

export interface ForwardRun {
  id: string;
  paradigm_version_id: string;
  start_date: string;
  end_date?: string;
  status: string; // active / completed / stopped
  initial_cash: number;
  final_cash: number;
  final_position_value: number;
  total_pnl: number;
  total_return: number;
  signal_count: number;
  filled_count: number;
  rejected_count: number;
  executed_count: number;
  max_drawdown: number;
  win_rate: number;
  sharpe_ratio: number;
  positions: Record<string, ForwardPositionState>;
  equity_curve: EquityPoint[];
  constraints_snapshot: ConstraintsSnapshot;
  created_at: string;
  updated_at: string;
}

export interface DataSnapshot {
  dataset_id: string;
  feature_set_id: string;
  rule_set_id: string;
  data_hash: string;
  captured_at: string;
}

export interface SignalSource {
  rule_id: string;
  rule_desc: string;
  triggered_by: string;
  context_tags?: Record<string, string>;
}

export interface ExecutionRecord {
  status: string; // pending / filled / partial / rejected / cancelled
  market: ExecutionMarket;
  exec_price: number;
  exec_qty: number;
  fee: number;
  gross_pnl: number;
  pnl: number;
  hold_qty: number;
  hold_cost: number;
  reject_reason?: string;
  executed_at: string;
}

export interface ExecutionMarket {
  date: string;
  open: number;
  high: number;
  low: number;
  close: number;
  pre_close: number;
  volume: number;
  amount: number;
  limit_up: number;
  limit_down: number;
  suspended: boolean;
  board: string;
}

export interface SignalEntry {
  id: string;
  run_id: string;
  paradigm_version_id: string;
  stock_code: string;
  direction: string;
  signal_date: string;
  execution_date?: string;
  price: number;
  pre_close: number;
  limit_up: number;
  limit_down: number;
  suspended: boolean;
  board: string;
  market: ExecutionMarket;
  confidence: number;
  data_snapshot: DataSnapshot;
  source: SignalSource;
  execution?: ExecutionRecord;
  content_hash: string;
  created_at: string;
}

export interface ForwardRunCreateRequest {
  paradigm_version_id: string;
  start_date: string;
  initial_cash: number;
  enable_t_1?: boolean;
  enable_price_limit?: boolean;
  enable_suspension?: boolean;
  board?: string;
  commission_rate?: number;
  slippage_bps?: number;
  stamp_duty_rate?: number;
}

export interface ForwardRunExecuteRequest {
  from_date?: string;
  to_date?: string;
  signal_id?: string;
}

export interface ForwardRunExecuteResponse {
  executed: number;
  rejected: number;
  results?: ExecutionRecord[];
}

export interface ForwardRunCompareRequest {
  theoretical_return: number;
  theoretical_max_drawdown: number;
  theoretical_sharpe: number;
  theoretical_win_rate: number;
  theoretical_signals: number;
  theoretical_annualized_return: number;
}

export interface TheoreticalMetrics {
  total_return: number;
  annualized_ret: number;
  max_drawdown: number;
  sharpe_ratio: number;
  win_rate: number;
  signal_count: number;
  ideal_pnl: number;
}

export interface ActualMetrics {
  total_return: number;
  annualized_ret: number;
  max_drawdown: number;
  sharpe_ratio: number;
  win_rate: number;
  signal_count: number;
  filled_count: number;
  rejected_count: number;
  actual_pnl: number;
  missed_trades: number;
}

export interface GapAnalysis {
  return_gap: number;
  return_gap_pct: number;
  drawdown_gap: number;
  sharpe_gap: number;
  win_rate_gap: number;
  exec_loss: number;
  constraint_impact: number;
  key_insights: string[];
}

export interface ComparisonReport {
  paradigm_version_id: string;
  run_id: string;
  compare_date: string;
  theoretical: TheoreticalMetrics;
  actual: ActualMetrics;
  gap: GapAnalysis;
}

export interface EquityPoint {
  date: string;
  cash: number;
  value: number;
  total: number;
}

// ============================================================================
// 监控模块类型
// ============================================================================

export interface DriftDetectionResult {
  type: string;
  significant: boolean;
  severity: string;
  metric_name: string;
  old_value: number;
  new_value: number;
  delta: number;
  delta_pct: number;
  p_value: number;
  threshold: number;
  sample_size: number;
  description: string;
}

export interface DriftSummary {
  total_detections: number;
  significant_count: number;
  severe_count: number;
  average_delta_pct: number;
  overall_status: string;
}

export interface DecayDetectionResult {
  type: string;
  is_decaying: boolean;
  severity: string;
  current_value: number;
  historical_avg: number;
  change_pct: number;
  window_days: number;
  confidence: number;
  description: string;
  detected_at: string;
}

export interface DecaySummary {
  total_detections: number;
  decaying_count: number;
  critical_count: number;
  avg_confidence: number;
  overall_status: string;
}

export interface ConcentrationResult {
  type: string;
  hhi: number;
  effective_count: number;
  is_concentrated: boolean;
  severity: string;
  top_contributor: string;
  top_weight: number;
  threshold: number;
  breakdown: Record<string, number>;
  description: string;
}

export interface ConcentrationSummary {
  total_detections: number;
  concentrated_count: number;
  critical_count: number;
  avg_hhi: number;
  overall_status: string;
}

export interface AlertItem {
  id: string;
  category: string;
  level: string;
  status: string;
  title: string;
  message: string;
  source: string;
  metric_name: string;
  metric_value: number;
  threshold: number;
  metadata: Record<string, unknown>;
  created_at: string;
  updated_at: string;
  acked_at?: string;
  resolved_at?: string;
  acked_by?: string;
}

export interface AlertSummary {
  total_alerts: number;
  active_count: number;
  acked_count: number;
  resolved_count: number;
  suppressed_count: number;
  critical_count: number;
  danger_count: number;
  warning_count: number;
  info_count: number;
}

export interface MonitoringReport {
  source: string;
  generated_at: string;
  period: {
    start_date: string;
    end_date: string;
    window_days: number;
  };
  drift_results: DriftDetectionResult[];
  drift_summary: DriftSummary;
  decay_results: DecayDetectionResult[];
  decay_summary: DecaySummary;
  concentration_results: ConcentrationResult[];
  concentration_summary: ConcentrationSummary;
  generated_alerts: AlertItem[];
  alert_summary: AlertSummary;
  recommendations: string[];
  health_score: number;
}

export interface PositionItem {
  code: string;
  name?: string;
  industry: string;
  weight: number;
  value?: number;
}

export interface MonitoringAlertAckRequest {
  user?: string;
}

// ============================================================================
// 监控模块类型
// ============================================================================

export interface DriftDetectionResult {
  type: string;
  significant: boolean;
  severity: string;
  metric_name: string;
  old_value: number;
  new_value: number;
  delta: number;
  delta_pct: number;
  p_value: number;
  threshold: number;
  sample_size: number;
  description: string;
}

export interface DriftSummary {
  total_detections: number;
  significant_count: number;
  severe_count: number;
  average_delta_pct: number;
  overall_status: string;
}

export interface DecayDetectionResult {
  type: string;
  is_decaying: boolean;
  severity: string;
  current_value: number;
  historical_avg: number;
  change_pct: number;
  window_days: number;
  confidence: number;
  description: string;
  detected_at: string;
}

export interface DecaySummary {
  total_detections: number;
  decaying_count: number;
  critical_count: number;
  avg_confidence: number;
  overall_status: string;
}

export interface ConcentrationResult {
  type: string;
  hhi: number;
  effective_count: number;
  is_concentrated: boolean;
  severity: string;
  top_contributor: string;
  top_weight: number;
  threshold: number;
  breakdown: Record<string, number>;
  description: string;
}

export interface ConcentrationSummary {
  total_detections: number;
  concentrated_count: number;
  critical_count: number;
  avg_hhi: number;
  overall_status: string;
}

export interface AlertItem {
  id: string;
  category: string;
  level: string;
  status: string;
  title: string;
  message: string;
  source: string;
  metric_name: string;
  metric_value: number;
  threshold: number;
  metadata: Record<string, unknown>;
  created_at: string;
  updated_at: string;
  acked_at?: string;
  resolved_at?: string;
  acked_by?: string;
}

export interface AlertSummary {
  total_alerts: number;
  active_count: number;
  acked_count: number;
  resolved_count: number;
  suppressed_count: number;
  critical_count: number;
  danger_count: number;
  warning_count: number;
  info_count: number;
}

export interface MonitoringReport {
  source: string;
  generated_at: string;
  period: {
    start_date: string;
    end_date: string;
    window_days: number;
  };
  drift_results: DriftDetectionResult[];
  drift_summary: DriftSummary;
  decay_results: DecayDetectionResult[];
  decay_summary: DecaySummary;
  concentration_results: ConcentrationResult[];
  concentration_summary: ConcentrationSummary;
  generated_alerts: AlertItem[];
  alert_summary: AlertSummary;
  recommendations: string[];
  health_score: number;
}

export interface PositionItem {
  code: string;
  name?: string;
  industry: string;
  weight: number;
  value?: number;
}

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

export interface CodeItem {
  Code: string;
  Name: string;
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

export interface ScreenResponse {
  results: ScreenResult[];
  total: number;
  matched?: number;
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

export interface SignalAnalysis {
  code: string;
  type: string;
  count: number;
  signals: number;
  outcomes: SignalOutcome[];
  summary: SignalSummary[];
}

export interface HistoryStock {
  code: string;
  analyzed_at: string;
}

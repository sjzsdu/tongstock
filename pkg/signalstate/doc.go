// Package signalstate 定义「状态型」技术态模型：每只股票每个交易日有 4 个状态量，
// 每个状态量取 买入 / 卖出 / 中性 三态之一（数据不足时另有独立态 insufficient_data）。
//
// 它与 pkg/signal 的事件型模型（signal.Signal：某天发生了一个金叉事件）互不替代：
// 事件描述「发生了什么」，状态描述「今天处于什么位置」，后者才是刻画股票并观察
// 状态变迁与股价关系的基础数据。
//
// 本包是维度、三态、以及「哪个态算买入哪个态算卖出」的唯一事实源。前端、接口、
// 后续各期的筛选与回溯都必须从这里派生，禁止在别处再写一套方向映射
// （历史上 web/src/types/screen.ts 的 SIGNAL_OPTIONS 就是这么漂移的）。
//
// 内核是纯函数：只依赖传入的前缀 K 线（klines[0..t]），不允许 time.Now、网络、
// 数据库，也不允许 signal.Detect 的事件语义与 filterSignalsByTrend 的窗口末端
// 趋势过滤（后者用「未来」的趋势筛「历史」的信号，是未来函数）。
package signalstate

package selection

// ExportRunHashForVersion 让 selection_test 能算出「某个历史 EngineVersion 下
// 落库的 run_hash」，用来构造一条真实存在的旧缓存记录，验证版本 bump 后它不会
// 再被命中。仅测试编译期可见（本文件不参与构建）。
var ExportRunHashForVersion = runHashForVersion

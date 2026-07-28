# 统一股票数据服务

`internal/app/stockdata.Service` 是日 K、行情和财务数据的应用服务。CLI 与 HTTP 都构造 `DataRequest` 调用它；`pkg/tdx.Service` 是上游协议适配器，不再承担最终的一致性语义。

```go
result, err := service.Query(ctx, stockdata.DataRequest{
    Spec: stockdata.DataSpec{
        Type: stockdata.DataKline,
        Market: "sz",
        Code: "000001",
        Granularity: "day",
        KType: 9,
        Start: start,
        End: end,
    },
    Mode: stockdata.RequireFresh,
})
```

查询过程固定为：检查数据库覆盖 → 新鲜度决策 → 必要时从 Provider 同步 → 校验 → 事务写入业务表和水位 → 重新读库。Provider 返回的内存对象不会直接返回给调用方。

`FreshnessPolicy`、`TradingCalendar` 和 `Clock` 都可注入。测试覆盖盘前、盘中、盘后、周末/节假日、未来日期、Kline 局部缺口、财务 TTL、并发合并、等待者取消和同步失败回滚。

稳定错误码包括：

| code | 含义 |
|---|---|
| `validation_failed` | 请求或模式无效 |
| `cache_miss` | cache-only 下数据库没有数据 |
| `upstream_unavailable` | TDX 同步失败 |
| `upstream_timeout` | TDX 同步超时 |
| `persistence_failed` | 数据库检查、事务或重读失败 |
| `stale_data` | 无法满足新鲜度要求 |

非日线 Kline、分钟、分笔和 F10 等尚未迁入统一 read model 的能力继续走类型化 `pkg/tdx.Service`，但 HTTP/CLI 禁止直接持有或调用裸 `tdx.Client`。

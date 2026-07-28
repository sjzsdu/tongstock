# 关键架构图

## 请求与同步

```text
CLI ─────┐
         ├──> DataRequest ──> stockdata.Service ──> SQLite Repository
HTTP ────┘                           │                    │
                                    │ stale/missing      │ fresh
                                    v                    v
                              TDX Provider           Query DB
                                    │
                              validate dataset
                                    │
                        transaction(data + watermark)
                                    │
                               re-query DB
                                    │
                                  result
```

## 服务生命周期

```text
config
  └─ storage + migrations
       └─ TDX executor
            └─ application services
                 └─ business stores / optional modules
                      └─ HTTP router + server + listener

Shutdown:
listener/HTTP → background jobs → services → TDX executor → storage
```

## HTTP 横切能力

```text
request
  → request_id
  → structured access log
  → panic recovery
  → security headers / body limit
  → stable error envelope
  → route module
```

SSE 在响应头发送前返回标准 JSON 错误；开始流式响应后发送结构化 `error` event。

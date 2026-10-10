
## 2026-10-08 范式挖掘会话备注
- 用户(cron)定期发起个股范式挖掘: 120交易日窗口, 要求完整分析+末尾机器可解析JSON(固定schema, operator仅限gt/lt/between/near/cross_above/cross_below/describe), 严禁编造回测/胜率/样本量/置信度。
- 已完成: 600519(截至9/30), 601688(截至10/8)。
- CLI坑: 本机安装版无 news 命令; indicator --json 的 --days 控制返回天数(默认70), --count 只影响计算窗口; kline 默认仅返回100根; HTTP server(:8080)未运行。
- F10 company-content 的 --block 参数多数返回同一份"股东研究"内容, 定位其他栏目需用 --start offset(目录由 company 命令给出)。
- web_search(Sogou)高频调用会403限流。
- 画像字段常见错误: 市值规模估算偏低(600519标mid实为1.57万亿; 601688标large实为约1605亿), finance接口数值单位为千元(如净利11691539千元=116.9亿)。

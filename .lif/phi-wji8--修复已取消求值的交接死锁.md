---
# phi-wji8
title: 修复已取消求值的交接死锁
status: completed
type: task
priority: normal
created_at: 2026-10-31T13:23:11Z
updated_at: 2026-10-31T13:25:06Z
---

确保运行循环丢弃已取消的未启动请求时不等待已经离开的调用方，并验证后续求值可继续。

## Summary of Changes

丢弃未启动的已取消请求时不再向离开的调用方发送响应；回归测试验证后续求值可继续，竞态测试及 make test、make check 通过。

---
# phi-k88q
title: 验证最小根文件系统中的 Glojure 求值
status: completed
type: task
priority: normal
created_at: 2026-10-31T12:55:52Z
updated_at: 2026-10-31T12:58:15Z
---

在独立 user、PID、network 命名空间及仅包含测试二进制与必要动态库的根视图中验证 Glojure 运行、取消和文件不可见；记录失败边界。

## Summary of Changes

以静态测试二进制和最小根视图验证 Glojure 求值、宿主文件不可见及阻塞夹具启动；重复十次，make test 与 make check 通过。已记录未证明后代退出等限制。

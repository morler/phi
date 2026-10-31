---
# phi-s8ed
title: 验证 Glojure 对命名空间外工作的影响
status: completed
type: task
priority: normal
created_at: 2026-10-31T12:37:41Z
updated_at: 2026-10-31T12:40:43Z
---

在隔离进程内由 Glojure 实际发起宿主操作，验证取消后已委托外部服务的工作与子进程生命周期，记录可复现边界。

## Summary of Changes

以真实 Glojure 宿主调用验证命名空间内子进程随 worker 断连，以及命名空间外 HTTP 服务在 worker 终止后仍可完成操作；记录边界并通过重复实验、make test 和 make check。

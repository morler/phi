---
# phi-9u6b
title: 跟踪隔离实例的派生进程退出
status: completed
type: task
priority: normal
created_at: 2026-10-31T13:01:20Z
updated_at: 2026-10-31T13:05:47Z
---

在最小根视图中由 Glojure 派生独立进程组子进程，通过宿主可见 PID 和 pidfd 验证取消后 worker 及子进程均退出。

## Summary of Changes

在最小根视图中由 Glojure 启动独立进程组子进程，使用 pidfd 确认 worker 与该子进程取消前存活、取消后均退出；重复十次并通过 make test 和 make check。

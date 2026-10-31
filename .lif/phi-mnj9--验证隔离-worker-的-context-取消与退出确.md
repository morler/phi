---
# phi-mnj9
title: 验证隔离 worker 的 context 取消与退出确认
status: completed
type: task
priority: normal
created_at: 2026-10-31T13:10:31Z
updated_at: 2026-10-31T13:12:05Z
---

使用 context 取消隔离 worker 的启动进程，并在返回前通过 pidfd 验证 worker 及其独立进程组子进程退出。

## Summary of Changes

测试改为真实 context 取消启动进程，等待启动进程及通过 pidfd 分别确认 worker 和另建进程组子进程退出；重复十次，make test 和 make check 通过。

---
# phi-s3m7
title: 验证隔离 worker 在取消后真正退出
status: completed
type: task
priority: normal
created_at: 2026-10-31T12:59:00Z
updated_at: 2026-10-31T13:00:43Z
---

使用可验证进程身份跟踪 Bubblewrap 内部 worker，取消后确认其退出，不仅回收启动进程。

## Summary of Changes

使用 Bubblewrap info-fd 和 Linux pidfd 跟踪报告的子进程，验证终止启动进程后该子进程退出；重复 20 次，make test 和 make check 通过，记录后代及外部副作用限制。

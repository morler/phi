---
# phi-snbt
title: 排除取消实验中的自然死锁退出
status: completed
type: task
priority: normal
created_at: 2026-10-31T13:14:03Z
updated_at: 2026-10-31T13:15:53Z
---

确保 Glojure 阻塞夹具在取消前保持存活，并核验实际 worker 的进程身份和退出。

## Summary of Changes

将阻塞夹具改为有界等待，取消前以 pidfd 确认实际 worker 存活，取消后确认退出；重复十次，make test 和 make check 通过。

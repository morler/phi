---
# phi-5oge
title: 验证受限文件系统视图与 Unix socket
status: completed
type: task
priority: normal
created_at: 2026-10-31T12:43:57Z
updated_at: 2026-10-31T12:46:36Z
---

对比只读挂载和隐藏目录对 Glojure 宿主调用、Unix socket 连接的影响，记录可复现限制。

## Summary of Changes

对比只读挂载、tmpfs 遮蔽与显式继承 socket：只读路径 socket 仍可连接，遮蔽阻断路径，继承连接仍可通信；实验重复十次并通过 make test 与 make check。

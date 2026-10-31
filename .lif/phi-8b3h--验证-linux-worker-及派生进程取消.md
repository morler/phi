---
# phi-8b3h
title: 验证 Linux worker 及派生进程取消
status: completed
type: task
priority: normal
created_at: 2026-10-31T12:31:04Z
updated_at: 2026-10-31T12:33:19Z
---

通过受控独立进程验证 Glojure worker、同组后代及脱离进程组的后代在取消时的行为，并可靠回收实验进程和记录限制。

## Summary of Changes

Linux 隔离实验验证同组后代会随进程组终止，脱离进程组的后代仍可写文件；监督进程回收实验后代。重复 20 次及 make test、make check 通过，限制已记录。

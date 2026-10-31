---
# phi-a8ch
title: 验证 PID 命名空间取消边界
status: completed
type: task
priority: normal
created_at: 2026-10-31T12:34:40Z
updated_at: 2026-10-31T12:36:56Z
---

验证 Linux PID 命名空间及其监督进程退出时能否终止脱离进程组的 Glojure worker 后代，检查启动与平台限制，记录证据。

## Summary of Changes

Linux PID/user 命名空间实验覆盖主动取消及监督进程被杀；同组与另建进程组的后代管道关闭且未写标记，重复 20 次通过。记录部署及安全限制，make test 和 make check 通过。

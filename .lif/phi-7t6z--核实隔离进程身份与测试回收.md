---
# phi-7t6z
title: 核实隔离进程身份与测试回收
status: completed
type: task
priority: normal
created_at: 2026-10-31T13:06:44Z
updated_at: 2026-10-31T13:09:30Z
---

核对 Bubblewrap 进程身份与 Glojure 派生进程的关系，修复实验中不可靠的进程发现及失败时清理。

## Summary of Changes

纠正 Bubblewrap child-pid 身份误判；通过线程子进程列表及 NSpid 核验真实 Glojure 派生进程，pidfd 验证退出并修复失败清理。重复十次，make test 和 make check 通过。

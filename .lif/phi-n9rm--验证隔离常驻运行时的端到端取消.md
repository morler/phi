---
# phi-n9rm
title: 验证隔离常驻运行时的端到端取消
status: completed
type: task
priority: normal
created_at: 2026-10-31T12:53:30Z
updated_at: 2026-10-31T12:55:09Z
---

以实际 Glojure 常驻实例和隔离子进程验证多次求值、状态保留、取消、进程结束及未提交状态丢失。

## Summary of Changes

隔离进程内运行实际常驻 Glojure，验证连续求值保留定义、阻塞求值时终止并回收、重启后定义丢失；实验重复 20 次，make test 和 make check 通过。

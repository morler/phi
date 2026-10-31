---
# phi-r4p9
title: 区分排队取消与已开始的求值
status: completed
type: task
priority: normal
created_at: 2026-10-31T13:19:52Z
updated_at: 2026-10-31T13:22:22Z
---

以明确的启动握手避免排队取消错误地弃用运行时，同时保持运行中取消的现有处理。

## Summary of Changes

新增无缓冲启动握手，取消发生在启动前时不弃用运行时且不开始该请求；确定性测试及竞态测试通过，make test 和 make check 通过。

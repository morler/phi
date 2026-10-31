---
# phi-zu8d
title: 实现隔离 worker 求值协议
status: in-progress
type: task
priority: normal
created_at: 2026-10-31T13:25:48Z
updated_at: 2026-10-31T13:47:21Z
---

实现仅供内部使用的 worker 求值协议，保留结果与状态并在取消时终止及等待进程；不开放模型调用。

已实现协议及直接子进程取消雏形；仍需实现 fail-closed 隔离启动并验证后代退出，当前任务保持进行中。

修复并发 Close 被活动求值阻塞的问题，添加握手测试；隔离启动与后代退出确认仍未完成。

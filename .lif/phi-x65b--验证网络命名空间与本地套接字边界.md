---
# phi-x65b
title: 验证网络命名空间与本地套接字边界
status: completed
type: task
priority: normal
created_at: 2026-10-31T12:41:09Z
updated_at: 2026-10-31T12:43:06Z
---

使用真实 Glojure 和受控本地 TCP、Unix 域套接字服务，验证 Linux 网络命名空间能否阻断对外委托并记录限制。

## Summary of Changes

实际 Glojure 网络探针验证独立网络命名空间阻断宿主 TCP，Glojure 启动的子进程仍可连接文件路径 Unix socket；抽象 Unix socket 的 Go 探针连接失败。实验重复十次及 make test、make check 通过。

# 计划 001：验证 Glojure 执行、取消和权限边界

> 本计划是阶段 0 的技术验证和决策交付，不向模型开放新工具。逐步执行验证；如果结果不支持安全接入，记录结果并停止，不以绕过权限检查完成实验。

## 状态

- 优先级：P0；规模：M；风险：高；依赖：无。
- 计划基线：Phi `6d793e5`，NodeCode `5b8af1e`；Glojure 使用本地 `../glojure`。开始前执行 `git diff --stat 6d793e5..HEAD -- internal/lisp internal/agent/executor.go internal/permission internal/tools go.mod`，如有变化，先核对下述现状并更新计划。
- 完成后将 `plans/README.md` 中本计划状态改为已完成并链接决策记录。

## 目标

确定模型调用 Glojure 所需的实际权限、取消、恢复和状态边界，为后续 eval 与持久化设计提供证据。最终交付为一份可复现的实验记录和明确的下一阶段接口草案；本计划不把实验结论预先当作已实现功能。

## 已确认的现状

- `internal/lisp/runtime.go` 的 `New` 通过 `sync.Once` 返回进程级实例；`Eval` 串行执行并保留定义和当前命名空间。返回 `Value`、`Stdout`、`Stderr`。
- 同文件的 `Eval` 注释和实现说明，执行开始后取消会使 runtime 不可用，代码可能继续运行并产生副作用。`internal/lisp/runtime_test.go:TestRuntimeCancellation` 使用子进程隔离此测试。
- `internal/agent/executor.go` 明确规定 `ExtensionPre → Gate → Run → ExtensionPost`；`internal/tools/tooldef/tool.go` 定义工具及其处理器；`internal/tools/tools.go` 分别组装默认和只读工具集。`internal/agent/executor_test.go` 已覆盖扩展修改与 Gate 的关系。
- `cmd/repl.go` 使用 Glojure 自带 REPL；它不是模型调用工具。`cmd/runloop.go` 的 headless 路径从 `DefaultTools` 选择工具。
- `go.mod` 将 `github.com/glojurelang/glojure` 替换到 `../glojure`。`../glojure/doc/interop.md` 描述 Go 宿主符号和 JVM 兼容层；可用符号、加载器与沙箱范围仍需以当前版本实际验证。
- NodeCode `../nodecode/src/README.md:91` 记载定义层跨重启加载；同文件 `:160` 说明 NodeCode eval 具有完整宿主权限。Phi 的权限约束不能由此推定为相同。
- Go 测试使用 `testify`；测试放在被测代码旁。执行实现时按仓库要求运行 `make fmt`、`make test`、`make check`；PXB 生成文件不得手改。

## 范围

允许新增临时、可重复运行的验证代码及其测试，优先放在 `internal/lisp` 附近；新增结论文件 `plans/002-execution-decision.md`，并更新 `plans/README.md`。如果验证需要改变 Phi 的工具注册、默认策略、PXB 协议、外部 Glojure 仓库或用户真实文件，停止并先调整设计范围。实验中允许独立子进程，但不得声称它自动提供宿主权限隔离。

本计划不实现模型工具、定义持久化、cells、gateway、自动 Git 提交或 NodeCode 源码兼容，也不改变 `go.mod` 的本地替换。实验必须使用临时目录及无凭据环境；绝不读取或输出用户密钥。

## 步骤

### 1. 固定可重复基线

记录三个工作树的 commit 和状态：`git rev-parse --short HEAD`、`git -C ../nodecode rev-parse --short HEAD`、`git -C ../glojure rev-parse --short HEAD`、各自 `git status --short`。不得清理现有工作区。运行 `go test ./internal/lisp ./internal/agent ./internal/tools`，记录命令、退出状态和失败原因；若基线失败，先报告，不将其归因于后续实验。

核对目标行为：阅读 NodeCode `src/README.md` 的 `One surface`、`The layer`、`Containment`，以及 `src/src/engine/turn.lisp` 中 eval 与 `src/src/organism/scribe.lisp` 中定义记录相关实现，提取可观察的成功、失败和恢复行为。记录来源文件及目标 commit，不复制 Common Lisp 实现作为 Glojure 代码。

**验证：** 上述 Git 命令成功；基线测试退出 0，或者决策记录明确记录失败并阻止后续实现。

### 2. 实测宿主互操作与权限面

在 `internal/lisp` 的独立测试子进程中，以受控临时目录和空白环境验证：普通 Clojure 表达式及定义；当前已加载环境能否直接或间接访问临时文件、启动无害子进程及建立本地连接；能否通过命名空间加载或 Go/JVM 互操作到达同类能力。使用确定性本地夹具，连接仅指本机测试监听器，不访问外网。每个操作分别记录允许、拒绝或无法判断及证据；不要把几条失败样例当作所有绕过路径均已关闭。

对照 `internal/permission` 的 `Request`、模式与 `internal/agent/executor.go` 的调用链，写明两种候选：将整个 eval 定义为有宿主权限的高风险执行并做完整调用授权；或限制所有宿主能力并通过经过 Gate 的桥接执行细粒度操作。比较取消、现有只读/子代理策略、扩展链及跨平台可实施性，选择可验证方案；没有证据的能力标记未知。

**验证：** `go test ./internal/lisp` 退出 0；实验记录对每个测试列出代码位置、输入类别、预期和实测。拒绝或未知路径不得被标记为已隔离。

### 3. 实测取消与恢复

在独立子进程中验证：执行前取消不执行代码；执行中取消后观察现有 `ErrUnavailable` 和潜在继续执行行为；终止运行进程后再启动，进程内未持久化定义不可用；普通运行时异常后下一次求值的行为。使用明确的同步握手和有界超时，不依靠固定 sleep 判定进程已退出。涉及外部副作用的测试仅写临时目录，并说明终止不能撤销已经完成的副作用。

输出建议的运行位置、生命周期、并发与取消语义：至少区分正常完成、取消前、运行中取消、崩溃、重启和重新加载；给出状态丢失范围和需要的审计信息。若主张通过进程边界终止，必须验证目标平台的进程及其派生进程处理；未验证的平台明确标注未知。

**验证：** `go test ./internal/lisp -run 'TestRuntime|Test.*Cancellation'` 退出 0；新增子进程测试可重复执行 `go test ./internal/lisp -count=3`，均退出 0，无遗留进程。

### 4. 明确工具接入和定义持久化的合同

在决策记录中画出实际数据流：模型调用 → 扩展前处理 → 权限请求 → 执行 → 扩展后处理 → 输出；说明交互与 headless 的拒绝/询问行为，以及只读与子代理的默认不可用方式。包含扩展修改参数后必须重新按最终参数授权的现有顺序，且不得把 MCP schema 直接暴露给模型。

定义后续持久化协议的设计问题和首批范围：哪些 Clojure 顶层定义可提交、定义身份与命名空间、依赖顺序、失败或多表单部分成功时的状态、原子写入与崩溃恢复、重复加载、删除、版本回滚、损坏项隔离、运行时状态与磁盘权威的关系。明确重启时不得执行任意求值历史；对于尚无结论的宏、副作用和动态定义，列出验证用例，不宣布支持。

列出 NodeCode 中无法直接等价的能力：Common Lisp 到 Clojure 的语言差异、Go 静态代码热替换边界、NodeCode 全宿主权限与 Phi 细粒度权限的差异。各项标记「目标等价」「暂不支持」或「需实验」，并链接到后续阶段。

**验证：** `plans/002-execution-decision.md` 包含威胁边界、证据表、推荐方案、反例、失败与恢复、兼容差异、开放问题和下一阶段可执行验收；对应 `plans/README.md` 阶段 1–2 的依赖与结论一致。

### 5. 确认变更与交接

运行 `make fmt`、`make test`、`make check`。如果仅增加文档而没有新增实验代码，可跳过格式化和测试，但必须在决策记录注明原因；不能把未运行写成通过。运行 `git status --short`，核对没有改动计划范围外的文件或覆盖用户现有变更；更新 `plans/README.md` 状态。

**验证：** 运行过的检查均退出 0；未能运行的检查有原因和影响；无范围外修改。后续实现计划应引用本决策结果，并分别覆盖权限拒绝、扩展处理、TUI/headless、取消及重启。

## 停止条件

- 修改 Glojure 自身、现有权限规则或默认工具集才能完成本次验证。
- 无法证明提议的工具权限与 Phi 既有权限和子代理约束兼容；保留实验结果，不开放模型调用。
- 实验出现受控临时目录以外的副作用，或无法终止遗留进程。
- 基线已漂移且现状与上述合同不一致；先更新计划，不能按旧假设继续。

## 维护与后续

实验记录需保留 Glojure、Phi、NodeCode 版本，方便依赖升级时重跑。后续 eval 工具与持久化层分开立项：前者要有扩展链和权限集成测试，后者要有跨进程重启和故障注入测试。Glojure 的中断实现、可加载命名空间或宿主互操作发生变化时，重新评估此决策。

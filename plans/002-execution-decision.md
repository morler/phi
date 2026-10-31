# 执行边界决策记录

状态：进行中。[计划 001](001-glojure-execution-boundary.md) 的第 1、2 步已完成；取消与恢复实验尚未进行，不能据此开放模型调用。

## 1. 可重复基线

记录时间：2026-10-31。以下状态是在新增本决策记录之前取得的；没有清理工作区。

| 工作树 | `git rev-parse HEAD` | `git status --short` |
| --- | --- | --- |
| Phi (`.`) | `6d793e5f82ed9c17065684eef8c197a817317cf6` | `?? .beans.yml`、`?? .beans/`、`?? plans/` |
| NodeCode (`../nodecode`) | `5b8af1ecfc4195c9f30009e7ca0bc2a0709abc3d` | 无输出 |
| Glojure (`../glojure`) | `6897cd9fe257a9665665919b11cec53decd5abda` | 无输出 |

对每个工作树分别运行 `git rev-parse HEAD` 和 `git status --short`，命令均成功。Phi 的未跟踪目录中已有计划与任务记录，保留原样。

在 Phi 运行 `go test ./internal/lisp ./internal/agent ./internal/tools`：退出状态 **0**；三个包均通过。此处只记录基线，不代表后续实验已通过。

## 2. NodeCode 目标行为

以下是对 NodeCode commit `5b8af1ecfc4195c9f30009e7ca0bc2a0709abc3d` 的源码和文档核对，不是对 Phi 或 Glojure 的实测结论。

- **统一入口：** TUI、网页及 `nodecode rpc` 通过 gateway 的 HTTP 和 WebSocket 接入；RPC 从标准输入接收命令并向标准输出写帧。来源：`../nodecode/src/README.md:68`。
- **求值成功：** 一个片段可顺序求值多个顶层表达式，返回值和输出对调用者可见；成功的顶层定义可记录到 layer。来源：`../nodecode/src/src/engine/turn.lisp:2014`、`../nodecode/src/src/engine/turn.lisp:2114`。
- **求值失败和取消：** 一般错误返回带失败标记的文本；多表达式片段不是事务，错误会指出哪一项失败、哪些先前结果仍然生效以及哪些尚未执行。取消条件继续向外传播，不伪装成普通工具结果。来源：`../nodecode/src/src/engine/turn.lisp:2091`、`../nodecode/src/src/engine/turn.lisp:2114`。
- **定义记录及失败：** 成功求值后才尝试记录定义；记录保留原始文本、每个定义一个文件，按目录提交并在可用时记录持久事件。记录失败会在求值输出中报告，已成功的求值不会因此变成轮次错误，也不能推断已持久化。正在调用栈上的重定义先暂存，经过一次成功调用后才记录；首次调用失败时恢复被替代的定义。来源：`../nodecode/src/src/engine/turn.lisp:108`、`../nodecode/src/src/engine/turn.lisp:2014`、`../nodecode/src/src/organism/scribe.lisp:1007`。
- **重启与回退：** layer 的定义在下次启动时覆盖发布版；顶层遗忘操作移除对应文件，若发布版有同名定义则恢复它。升级后目标已移动或不存在的 layer 文件会被跳过并报告，发布版定义继续生效；layer 的 Git 历史可用于撤销。来源：`../nodecode/src/README.md:91`、`../nodecode/src/src/organism/scribe.lisp:935`、`../nodecode/src/src/organism/scribe.lisp:1007`。
- **权限边界：** NodeCode 的 eval 有完整宿主权限；新进程隔离崩溃，但本身不隔离文件和网络影响。来源：`../nodecode/src/README.md:160`。

以上行为是固定版本的对照目标；尚未验证 Glojure 是否提供相同能力。后续步骤须分别验证权限、取消和恢复，并记录实际结果。

## 3. 宿主互操作实验

实验日期 2026-10-31；基线版本见第 1 节，Go `go1.27.3 linux/amd64`。实验代码为 `internal/lisp/host_probe_test.go:22` 和 `internal/lisp/host_probe_test.go:81`。父测试创建临时工作目录和固定内容的文件，以独立测试进程运行解释器；子进程仅接收探针变量、指向该目录的 `HOME` 和 `TMPDIR`，不继承用户环境或凭据。进程探针只再次运行当前测试二进制内的无害夹具；网络探针只连接父测试创建的 `127.0.0.1` 临时端口。测试子进程有 30 秒超时。进程隔离和空白环境是实验控制条件，不是文件、进程或网络沙箱。

表中的「预期」是本次固定版本的回归断言，不是安全策略；「失败」只说明该输入未完成，不证明其他路径被拒绝或隔离。每个输入及断言均在上述测试函数中，固定文件和进程标记还由父进程检查。

| 输入类别与位置 | 预期 | 实测 |
| --- | --- | --- |
| `TestHostProbe/expression`：`(+ 20 22)` | 返回 `42` | 返回 `42` |
| `TestHostProbe/define` 和 `call-definition`：定义 `probe-double`，另一次求值调用 `(probe-double 21)` | 定义后返回 `42` | 成功；后一次求值返回 `42` |
| `TestHostProbe/core-file`：`(spit "core.txt" "core") (slurp "core.txt")` | 报 `unable to resolve host form: gio.writer` | 符合预期，第一表单失败，后续 `slurp` 未执行 |
| `TestHostProbe/namespace-load`：`(require '[glojure.go.io :as gio])` | 加载返回 `nil` | 成功 |
| `TestHostProbe/namespace-writer`：加载后调用 `(gio/writer "namespace.txt")` | 报 `EvalASTMaybeHostForm: gio/writer` | 符合预期；不能推断其他命名空间接口不可用 |
| `TestHostProbe/java-namespace`：加载 `clojure.java.io` | 报 `not found in load path` | 符合预期 |
| `TestHostProbe/go-write`：`(os.WriteFile "go.txt" (.getBytes "go") 384)` | 写入临时文件 `go` | 返回 `nil`；父进程读到 `go` |
| `TestHostProbe/go-read`：读取 `fixture.txt` 并转字符串 | 返回 `"fixture"` | 返回 `"fixture"` |
| `TestHostProbe/go-process`：`os:exec.Command` 执行测试二进制及指定的 `TestHostProbeHelper` | 启动无害子进程并写入临时标记 | 返回 `nil`；父进程读到 `process.txt` 内容 `process` |
| `TestHostProbe/jvm-file`：`(.mkdir (java.io.File. "jvm-dir"))` | 创建临时目录并返回 `true` | 返回 `true`；父进程确认目录存在 |
| `TestHostProbe/jvm-runtime`：`(Runtime/getRuntime)` | 报 `EvalASTMaybeHostForm: Runtime/getRuntime` | 符合预期；没有以此声称 JVM 进程路径被关闭 |
| `TestHostNetworkProbe`：`net.Dial` 连接临时监听器 | 报 `unable to resolve symbol: net.Dial` | 符合预期；该输入未建立连接 |
| `TestHostNetworkProbe`：`java.net.Socket.` 连接同一临时监听器 | 报 `unable to resolve symbol: java.net.Socket` | 符合预期；该输入未建立连接 |
| `TestHostNetworkProbe`：`net:http.Get` 请求同一临时监听器 | 返回 HTTP 响应且监听器收到请求 | 返回 `http.Response`；监听器读取请求并返回 200 |

实验中没有测试任意命名空间、全部 JVM 接口、通过宏或反射到达宿主的所有路径，其他平台也未验证；均标记为**未知**。Glojure `../glojure/pkg/glj/init-goimports.go:6` 加载生成的宿主注册表，Linux/amd64 的 `../glojure/pkg/gen/gljimports/gljimports_linux_amd64.go:1778`、`:1784` 分别注册 `os.WriteFile`、`os/exec.Command`；注册并不代替上表的实际运行结果。

验证：`make fmt` 和 `make fmt-check` 均退出 0；`env GOCACHE=/tmp/phi-go-cache go test ./internal/lisp -count=1 -v` 及格式化后 `env GOCACHE=/tmp/phi-go-cache go test ./internal/lisp` 均退出 0，其中受沙箱 `socket: operation not permitted` 限制的网络测试明确跳过；已获准在沙箱外运行 `env GOCACHE=/tmp/phi-go-cache go test ./internal/lisp -run '^TestHostNetworkProbe$' -v -count=1`，退出 0，网络测试实际通过。默认 Go 缓存位于只读目录，首次普通测试因 `read-only file system` 失败，后改用临时缓存；不将该失败算成测试通过。

## 4. 权限方案比较与当前决定

现有 `permission.Request` 包含工具、操作类型、路径和命令等元数据（`internal/permission/policy.go:56`）；`internal/tools/tooldef/permission.go:61` 从工具及最终参数构造请求。`internal/agent/executor.go:195` 先运行扩展前处理，再以处理后的参数调用 Gate/Ask，随后执行和扩展后处理。当前实测中的 Glojure 宿主调用直接在 `Runtime.Eval` 内发生，没有为每次文件、进程或 HTTP 操作生成对应请求或经过扩展链。单个外层授权无法看见代码运行时计算出来的资源，也不能保证既有路径、命令和网络规则逐项生效。

| 候选 | 权限与扩展 | 取消、子代理和跨平台 | 结论 |
| --- | --- | --- | --- |
| 整次 eval 作为拥有完整宿主权限的高风险调用，显式授权最终代码 | 能审计和授权整次调用，但不能声称执行内部的细粒度规则；完整授权与现有路径/命令限制的兼容性尚未证明。`KindExec` 的 bash 规则和默认值不能直接代表任意 Clojure 代码 | 当前进程内取消后代码仍可能运行；未来独立进程有望提供终止点，派生进程和平台细节待第 3 步验证。不得让只读及子代理获得此能力；现有策略是否能表达这一独立权限尚待设计与测试 | 作为仅面向明确接受完整宿主权限的未来模式的候选；目前不能启用，也不能宣称与现有细粒度策略等价 |
| 禁用全部宿主能力，仅通过 Gate 桥接细粒度操作 | 理论上每个桥接操作可经过扩展和 Gate；但当前 Go/JVM 互操作已有绕过桥接的实证，尚无完整的禁用机制或封闭性证明 | 即使桥接支持取消，也须限制未桥接的异步操作；只读和子代理安全依赖可验证的完全封闭。不同平台的可用符号、加载器和隔离机制均待验证 | 当前不可验证，不作为安全沙箱使用 |

当前决定是**不向模型开放 eval**。如后续要选择完整宿主权限模式，必须先证明其明确授权不违背用户现有权限和子代理限制，并覆盖扩展修改最终参数后的授权、交互与无询问模式；做不到就继续禁用。`internal/permission/mode.go:32` 的 readonly 会拒绝写入和需要询问的操作，但对已获 Allow 的 `KindExec` 有例外，因此仅声明 `KindExec` 不是只读保护。`internal/agent/child_spec.go:24` 中 explore/review 使用只读工具集和 readonly，worker 使用 headless-strict；`internal/agent/engine_runner.go:114` 将子代理角色与父权限求交。扩展可在执行前拦截或改参数，但不会自动拦截解释器内部宿主操作。取消和跨平台进程树处理仍属第 3 步，不能由本次实验推断。

# 执行边界决策记录

状态：进行中。[计划 001](001-glojure-execution-boundary.md) 的第 1–3 步已完成；模型调用仍未开放。

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

当前决定是**不向模型开放 eval**。如后续要选择完整宿主权限模式，必须先证明其明确授权不违背用户现有权限和子代理限制，并覆盖扩展修改最终参数后的授权、交互与无询问模式；做不到就继续禁用。`internal/permission/mode.go:32` 的 readonly 会拒绝写入和需要询问的操作，但对已获 Allow 的 `KindExec` 有例外，因此仅声明 `KindExec` 不是只读保护。`internal/agent/child_spec.go:24` 中 explore/review 使用只读工具集和 readonly，worker 使用 headless-strict；`internal/agent/engine_runner.go:114` 将子代理角色与父权限求交。扩展可在执行前拦截或改参数，但不会自动拦截解释器内部宿主操作。

## 5. 取消与恢复实验

2026-10-31，Go `go1.27.3 linux/amd64`，Glojure `6897cd9fe257a9665665919b11cec53decd5abda`，Phi 基于 `eb3dc21` 加本次测试。`internal/lisp/recovery_test.go` 在临时目录中启动独立测试进程，以测试注入的 Go 函数发出开始信号并等待释放；父进程收到握手后才终止 worker，所有等待均有超时，进程被 `Wait` 回收。另见已有 `TestRuntime` 和 `TestRuntimeCancellation`。

| 场景 | 实测结果 |
| --- | --- |
| 求值前取消 | 返回 `context.Canceled`，后续查询确认定义未创建；运行时仍可用。 |
| 求值中取消 | 收到开始信号后取消，调用返回 `context.Canceled` 和仍可能运行的提示；后续求值返回 `ErrUnavailable`。释放阻塞函数后，它仍写入临时文件，父进程验证了文件内容。 |
| 终止及重启 | worker 先定义变量并写临时文件，在阻塞求值中发出就绪信号；父进程杀死并回收 worker，已写文件仍存在。新进程中该变量无法解析。 |
| 普通异常 | 已有测试抛出异常并检查错误与 stderr，随后同一运行时仍可调用此前定义；读取错误后亦可继续求值。 |
| 并发 | 已有测试验证多个请求串行更新同一计数器；它不证明跨进程共享状态。 |

`GOCACHE=/tmp/phi-go-cache go test ./internal/lisp -run 'TestRuntime|Test.*Cancellation' -count=1 -v` 和 `GOCACHE=/tmp/phi-go-cache go test ./internal/lisp -count=3 -v` 均退出 0；本次网络探针也通过。该测试回收其直接子进程；没有验证派生进程树、崩溃转储、跨平台终止或持久化重新加载，不能据此声称终止进程能终止后代或撤销外部副作用。

未来可考虑每个运行实例由独立 worker 串行执行，主进程负责授权、生命周期和结果转发；这只是设计方向，尚非实现或隔离保证。取消前不执行；正常完成返回结果、输出和错误；执行中取消先标记结果未知，再按经验证的平台机制终止并回收 worker，不能复用其瞬时状态。崩溃同样使未提交内存状态丢失，已发生外部副作用保留。重启后的已提交定义只有在提交及重载协议实现并验证后才能恢复；加载失败须报告并阻止将部分状态视为完整恢复。审计至少记录请求及最终授权代码、权限决定、worker 和运行世代、执行及终止结果、定义提交版本与恢复失败；具体敏感信息处理须在实现时确定。派生进程处理和各目标平台仍须单独验证，完成前不得承诺可靠终止。

## 6. 后续接口与持久化合同草案

这些是后续实现的验收要求，不代表当前支持：

- 模型工具必须显式启用，TUI 与 headless 复用同一执行链；经 `ExtensionPre` 修改后的最终代码再经 Gate/Ask，执行后经过 `ExtensionPost`。只读与所有子代理不得获得 eval；无询问模式和拒绝必须有测试。现有权限无法表达或验证所需完整宿主授权时继续禁用。
- 结果需区分值、stdout、stderr、普通错误、权限拒绝、执行前取消、执行中取消及 worker 崩溃；取消或崩溃时不得暗示副作用已回滚。多表单部分成功需明确报告，不能作为事务处理。
- 持久化需先定义可提交的顶层形式、命名空间与名称组成的身份、依赖顺序、覆盖及删除语义；仅提交明确成功的定义，禁止启动时重放任意求值历史。宏、动态定义和带副作用形式暂不宣称支持，须分别测试或拒绝。
- 磁盘提交须原子化并能检测不完整或损坏项；重启加载、重复加载、部分失败、版本回滚及故障注入须可验证。须明确磁盘权威与运行时状态不一致时的处理，不将运行成功等同提交成功，也不把部分加载当作完整恢复。
- 后续分别建立工具权限与扩展链集成测试、TUI/headless 测试，以及跨进程重启和故障注入测试；先解决权限与终止的未决问题再开放 eval。

验证记录：`make fmt`、`GOCACHE=/tmp/phi-go-cache make test` 和 `GOCACHE=/tmp/phi-go-cache make check` 均退出 0。首次 `make check` 因已有网络探针的 `net.Listen` 触发 `noctx` 失败；改用带测试上下文的 `net.ListenConfig.Listen` 后重新通过。原有 `.beans` 删除和 `.lif` 新增是本次实验前已有的工作区状态，未清理；新增 lif 任务用于记录本次验证。

## 7. Linux 进程组取消实验

2026-10-31，Go `go1.27.3 linux/amd64`，Glojure `6897cd9fe257a9665665919b11cec53decd5abda`。`internal/lisp/process_linux_test.go` 使用独立测试监督进程，先设置 Linux child subreaper，再启动拥有独立进程组的 worker。worker 以 Go 测试夹具启动两个后代：一个继承进程组，另一个通过 `Setpgid` 新建进程组；随后 worker 在 Glojure 求值中发出就绪信号并阻塞。两个后代均等待监督进程的管道输入，不依赖固定 sleep。夹具模拟派生进程的行为，没有通过 Glojure 直接启动这两个后代；第 3 节已单独验证 Glojure 能启动进程。

监督进程收到两次派生和求值就绪信号后，向 worker 进程组发送 `SIGKILL` 并等待 worker 退出。同组后代被 `SIGKILL` 终止并由监督进程回收；脱离进程组的后代仍在运行，收到管道输入后写入临时文件并正常退出，由监督进程回收。测试退出路径关闭管道并等待已接管后代，不把僵尸进程当作运行中的进程。`GOCACHE=/tmp/phi-go-cache go test ./internal/lisp -run '^TestWorkerProcessGroup$' -count=20 -v`、`make fmt`、`GOCACHE=/tmp/phi-go-cache make test` 和 `GOCACHE=/tmp/phi-go-cache make check` 均退出 0。

结论：进程组信号对本实验的 worker 和同组后代有效，但不能保证终止脱离进程组的后代；现有 Bash 工具的进程组方式不能直接作为任意 eval 的完整终止保证。测试专用 subreaper 只负责接管和回收，不负责约束或自动终止脱离进程组的程序，亦未实现于产品。下一步须研究并验证能够覆盖派生和脱离行为的系统隔离机制及其部署前提，或明确限制宿主进程能力；在此之前继续禁止模型 eval。其他平台、外部服务和已发生的文件或网络副作用均未验证，不能承诺回滚。

## 8. Linux PID 命名空间实验

2026-10-31，Go `go1.27.3 linux/amd64`，内核 `7.2.6.p03.32-1.fc44.x86_64`；Glojure 与第 7 节相同。`internal/lisp/pidnamespace_linux_test.go` 通过 Go `SysProcAttr` 创建用户和 PID 命名空间，让 worker 成为该命名空间的 PID 1；沿用第 7 节的同组和另建进程组后代夹具。调用方在收到两个后代和求值就绪信号后终止 worker 并 `Wait`；管道读端全部关闭，随后写入返回 `EPIPE`，两个后代均没有写出标记。该结果表明本次实验中两个持管道的后代均已关闭文件描述符，不单独证明所有可能的后代都已回收。

另一测试在独立监督进程中设置 `Pdeathsig=SIGKILL` 启动同样的命名空间 worker；外层收到握手后杀死并回收监督进程，同样观察到管道读端全部关闭和 `EPIPE`。这是监督进程被杀且启动线程仍存活时的观察；Linux 的父进程死亡信号与创建线程生命周期相关，启动竞态、正常退出、信号屏蔽和复杂父进程关系尚需独立验证。实验不证明异步系统服务、文件或网络副作用可以撤销，也没有证明不可信代码无法借助可访问的宿主资源在命名空间之外启动工作。进程隔离不是文件、网络或权限沙箱。

非特权用户命名空间可在本机创建，但部署环境可能禁止；测试在 `EPERM`、`EACCES` 或 `ENOSYS` 时明确跳过。禁止降级到不受控的进程内执行。`GOCACHE=/tmp/phi-go-cache go test ./internal/lisp -run '^TestWorker(PIDNamespace|SupervisorDeath)$' -count=20 -v`、`make fmt`、`GOCACHE=/tmp/phi-go-cache make test` 和 `GOCACHE=/tmp/phi-go-cache make check` 均退出 0，本机两项测试均实际运行，没有跳过。

PID 命名空间是值得继续验证的 Linux 方案，并非现成产品实现。下一步须用实际 Glojure 宿主进程调用、取消与失败恢复的完整执行链验证，核实命名空间外派生路径及其他平台，明确未支持环境的拒绝行为，并在权限方案证明前继续禁用模型 eval。

## 9. Glojure 实际宿主调用与外部委托

2026-10-31，环境同第 8 节。`internal/lisp/delegation_linux_test.go` 的两个独立测试均在临时目录、独立 PID/user 命名空间中运行实际 Glojure 求值；本机没有跳过。测试服务器只监听本机临时端口，使用握手而非固定时间决定何时终止 worker。

- `TestNamespaceGlojureProcess` 通过 Glojure `os:exec.Command` 启动测试子进程；子进程连接命名空间外的本地夹具并等待响应。收到连接后杀死并回收 worker，服务端观察到子进程连接断开，子进程未写出其后续标记。此测试覆盖实际 Glojure 启动的普通子进程；第 8 节的另建进程组后代仍由 Go 夹具创建。
- `TestNamespaceExternalDelegation` 由 Glojure 通过 `net:http.Get` 调用命名空间外的本地服务；服务确认收到请求后等待，worker 被杀并回收后再完成临时文件写入。服务端明确不因客户端断开而取消已接收工作，因此该实验证明终止求值进程无法撤回外部服务已接受的操作。

`GOCACHE=/tmp/phi-go-cache go test ./internal/lisp -run '^TestNamespace(GlojureProcess|ExternalDelegation)$' -count=20 -v` 两项各通过 20 次；`make fmt`、`GOCACHE=/tmp/phi-go-cache make test`、`GOCACHE=/tmp/phi-go-cache make check` 最终均退出 0。

结论：解决取消后的执行须区分可控 worker 与已提交的外部操作。PID 命名空间可作为 Linux worker 终止机制的候选；限制或代理网络、Unix socket、其他 IPC 等对外委托途径才可能限制未授权的后续外部工作，且仍不能撤销已提交操作。现有进程级取消没有实现这种完整边界；不能以此开放模型 eval 或承诺取消是事务回滚。下一步需要实测如何封闭网络及本地 IPC、确认所需正常能力的受控桥接，并设计无法启用隔离时的拒绝路径；其他平台仍未知。

## 10. 网络命名空间与 Unix socket 实验

2026-10-31，Linux 环境同第 8 节。`internal/lisp/network_namespace_linux_test.go` 在 PID/user/network 命名空间中运行测试 worker，父进程在宿主命名空间建立临时 TCP、文件路径 Unix socket 和抽象 Unix socket 监听器。测试使用独立进程与临时目录；命名空间不可创建时标记跳过，不回退到未隔离执行。

- Glojure 的 `net:http.Get` 访问宿主本地 TCP 监听器时返回非空 Go 错误，宿主监听器超时未收到连接。
- Glojure 通过 `os:exec.Command` 启动的测试子进程成功连接宿主的文件路径 Unix socket，宿主监听器实际接受连接。这里由 Go 子进程完成 Unix 连接，不声称 Glojure 直接暴露了 `net.Dial`；此前已验证解释器可启动宿主进程。
- 同一个网络命名空间内的 Go 夹具连接宿主抽象 Unix socket 失败；这是该夹具的观测，不是所有 IPC 路径均被封闭的证明。

`GOCACHE=/tmp/phi-go-cache go test ./internal/lisp -run '^TestNetworkNamespaceBoundary$' -count=10 -v` 实际运行且十次通过；`make fmt`、`GOCACHE=/tmp/phi-go-cache make test` 和 `GOCACHE=/tmp/phi-go-cache make check` 均退出 0。网络命名空间不能替代文件系统隔离：任何仍可见且有权限访问的文件路径 socket 都可能成为向外委托的通道。还须考虑继承的文件描述符、共享文件、其他 IPC 和服务端已经接受的操作；这些均未由此测试封闭。下一步应验证受限挂载与文件系统视图以及必要的受控桥接，并对创建隔离失败的情况保持拒绝，不能声称当前具备完整沙箱。

## 11. 挂载视图与继承描述符实验

2026-10-31，Linux 环境同第 8 节，使用本机 `bubblewrap 0.12.0` 作实验工具，不是产品依赖或已实现方案。`internal/lisp/mount_boundary_linux_test.go` 创建宿主临时目录及 Unix socket，启动带 user、PID、network 命名空间的进程，比较根目录只读绑定与将实验目录替换为独立 tmpfs 两种视图。Glojure `os.WriteFile` 在只读绑定中返回错误且宿主文件保持原值；在 tmpfs 中写入成功但没有改变宿主文件。Glojure 启动的子进程在只读绑定中仍可连接宿主 socket；目录被 tmpfs 遮住后连接失败，宿主端仅收到前一次连接。

另一个 Go 夹具将已连接 Unix socket 作为额外描述符传入 bubblewrap 进程；即使启用上述命名空间并遮住临时目录，夹具仍能通过该描述符向宿主端写入数据。这是显式传入描述符的反例，不代表默认启动会传入哪些描述符，也不证明其余 IPC 已封闭。测试故意只覆盖临时实验目录，不宣称只读绑定整个宿主根目录是安全方案。

`GOCACHE=/tmp/phi-go-cache go test ./internal/lisp -run '^TestMount(Boundary|InheritedSocket)$' -count=10 -v` 两项各实际通过十次；`make fmt`、`GOCACHE=/tmp/phi-go-cache make test` 和 `GOCACHE=/tmp/phi-go-cache make check` 均退出 0。没有安装 bubblewrap 时测试跳过；已安装但初始化失败时测试失败，不作为隔离成功。未来 worker 启动须明确最小文件系统视图和描述符白名单，对不可用隔离拒绝运行，并验证与正常工作区读写及现有 Gate 的兼容；任何已提交的外部操作仍无法撤销。

## 12. 只读工作区与现有权限的对照

2026-10-31，环境同第 11 节。`internal/lisp/workspace_boundary_linux_test.go` 在临时工作区创建普通文件与敏感文件，将后者配置进现有 `permission.Policy.SensitivePathDeny`，实际调用 Gate 确认普通文件允许读取、敏感文件拒绝读取。随后在 user/PID/network 命名空间及只读根挂载中运行 Glojure：两份文件均可通过 `os.ReadFile` 直接读出，写入普通文件返回错误且宿主内容不变。

因此即使完全只读，向解释器暴露整个工作区仍绕过现有的敏感路径拒绝；第 11 节还表明其中的路径 socket 可被连接。此实验使用只读绑定整个根目录作为反例，不能作为推荐的最小挂载配置。要维持现有逐项权限及扩展链，worker 不应直接接收可绕过规则的工作区访问；需通过宿主受控操作提供数据，且该通道必须在最终参数上执行 `ExtensionPre → Gate/Ask → Run → ExtensionPost`。桥接本身和并发取消、已提交副作用、描述符泄漏均尚未实现或验证。

`GOCACHE=/tmp/phi-go-cache go test ./internal/lisp -run '^TestReadonlyWorkspaceBypassesReadGate$' -count=20 -v` 实际通过二十次；`make fmt`、`GOCACHE=/tmp/phi-go-cache make test` 和 `GOCACHE=/tmp/phi-go-cache make check` 均退出 0。未安装 bubblewrap 时实验跳过，安装后初始化失败视为测试失败。

## 13. 权限检查期间的取消

现有 `Executor` 是未来受控桥接可复用的执行链，但原实现只在调用前检查取消：权限 Gate 或 Ask 返回允许时，即使等待期间上下文已取消，也会调用工具处理器。现已在扩展前处理后、Gate 返回后和 Ask 返回后检查取消，并在取得审批锁后检查取消，避免已取消的请求弹出过期审批或继续启动处理器。`internal/agent/executor_test.go` 的 `TestExecutorCancellationBeforeHandler` 覆盖 Gate 返回允许、Gate 返回询问及 Ask 返回批准时的取消；三种情况均不调用处理器并报告取消。

这只收窄取消发生在处理器启动前的窗口；检查和调用之间仍存在并发竞态，已开始的处理器也必须自行遵守上下文，外部已提交操作不能撤销。不能把该修复描述为完整的桥接取消或进程隔离方案。`GOCACHE=/tmp/phi-go-cache go test -race ./internal/agent -run '^TestExecutorCancellationBeforeHandler$' -count=10`、`make fmt`、`GOCACHE=/tmp/phi-go-cache make test` 和 `GOCACHE=/tmp/phi-go-cache make check` 均退出 0。

## 14. 常驻实例终止与状态实验

`internal/lisp/worker_lifecycle_linux_test.go` 在独立 PID/user 命名空间的测试进程中启动现有 `Runtime`，通过测试专用 JSON 行协议连续求值：先定义变量，再读取并得到 `42`；第三次求值通过测试注入的阻塞函数发出开始信号。父进程收到信号后杀死并回收 worker，确认输出管道关闭；新 worker 中同名变量无法解析。每次等待都有超时，测试不可创建命名空间时明确跳过。

该实验验证所测环境中常驻实例的进程生命周期和瞬时状态丢失；信号由测试主动发送，不是生产取消协议，阻塞函数为测试夹具，且没有文件系统、网络或桥接隔离保证。后续实现仍须处理请求提交与取消竞态、等待结束后才报告取消、启动失败拒绝、子进程与宿主操作、状态恢复和其他平台。`GOCACHE=/tmp/phi-go-cache go test ./internal/lisp -run '^TestIsolatedRuntimeLifecycle$' -count=20 -v` 实际运行通过二十次；`make fmt`、`GOCACHE=/tmp/phi-go-cache make test` 和 `GOCACHE=/tmp/phi-go-cache make check` 均退出 0。

## 15. 最小根视图实验

`internal/lisp/minimal_root_linux_test.go` 使用 `CGO_ENABLED=0` 构建测试二进制，以 bubblewrap 创建 user、PID、network、IPC 命名空间，只将二进制只读挂载到新根视图，并创建独立 `/proc`、`/dev` 和临时 `/tmp`。Glojure `(+ 20 22)` 返回 `42`，尝试读取宿主临时敏感文件返回错误。测试随后在同一视图启动第 14 节的阻塞运行时夹具，收到握手后杀死并回收 bubblewrap 主进程。

这证明本机上的静态测试二进制无需挂载工作区即可启动和求值；不证明全部语言功能、所有 IPC、子进程清理、权限链或生产取消语义。取消实验只等待 bubblewrap 主进程退出，没有独立确认内部 worker 及其所有后代已退出，因此不能将其当作取消后无副作用的证明。外部已提交操作仍不可回滚。`GOCACHE=/tmp/phi-go-cache go test ./internal/lisp -run '^TestMinimalRootRuntime$' -count=10 -v` 实际通过十次；`make fmt`、`GOCACHE=/tmp/phi-go-cache make test` 和 `GOCACHE=/tmp/phi-go-cache make check` 均退出 0。没有 bubblewrap 时测试跳过，构建或隔离失败时失败。

后续补充：测试通过 bubblewrap `--info-fd` 获取 `child-pid`，在终止启动进程前打开 Linux pidfd，之后等待 pidfd 可读，确认该报告的子进程确实退出，避免 PID 复用导致的误判。`GOCACHE=/tmp/phi-go-cache go test ./internal/lisp -run '^TestMinimalRootRuntime$' -count=20 -v` 实际通过二十次；`make fmt`、`GOCACHE=/tmp/phi-go-cache make test` 和 `GOCACHE=/tmp/phi-go-cache make check` 均退出 0。这修正了上段对该子进程退出状态未知的描述，但没有独立跟踪任意后代，也没有实现生产取消路径或证明取消后无外部副作用。测试要求内核支持 pidfd；不支持时报错而非默认为通过。

进一步补充及更正：此前将 bubblewrap 的 `child-pid` 当作 worker，并将其直接子进程当作 Glojure 启动的子进程，是错误的：本机该 PID 是命名空间内回收进程，其直接子进程才是 worker。旧测试因而只确认回收进程和 worker 退出，不能证明 Glojure 派生进程退出。

修正后的测试沿回收进程、worker 和 worker 的各线程的 `/proc` 子进程关系查找后代，并核对后代报告的命名空间内 PID 与宿主 `/proc/<pid>/status` 的 `NSpid`。Glojure 通过 `os:exec.Command` 启动的子进程另建进程组，父测试分别为 worker 和已核对身份的子进程打开 pidfd，确认二者取消前存活；终止并回收 bubblewrap 后，两者 pidfd 均变为可读。该用例重复十次通过。此结果覆盖一个真实 Glojure 启动且另建进程组的子进程，不证明任意代数后代、逃逸至外部服务的工作、生产取消和其他平台；已完成的外部操作依然不可撤销。

随后将该场景的启动进程改用 `exec.CommandContext`，在握手及 pidfd 存活检查后取消 context、等待启动进程退出，再等待 worker 和子进程的 pidfd 可读；重复十次通过。这里取消后 `Wait` 只等待 bubblewrap，后续单独等待 pidfd 才确认所测两个进程退出，不能假定 `Wait` 自身有此保证。`make fmt`、`GOCACHE=/tmp/phi-go-cache make test` 和 `GOCACHE=/tmp/phi-go-cache make check` 均退出 0。仍是测试专用控制流程，尚未提供生产 worker 或受控桥接。

夹具修正：第 14 节原阻塞函数使用空 `select`，Go 可能在取消前因死锁检测而退出。现改为有界等待，并在取消前通过 pidfd 确认实际 worker 仍存活；取消后再确认该 worker 的 pidfd 可读。`GOCACHE=/tmp/phi-go-cache go test ./internal/lisp -run '^(TestMinimalRootRuntime|TestIsolatedRuntimeLifecycle)$' -count=10 -v` 实际通过十次；`make fmt`、`GOCACHE=/tmp/phi-go-cache make test` 和 `GOCACHE=/tmp/phi-go-cache make check` 均退出 0。这仍不是生产取消协议。

## 16. 求值交接前的取消

进程内 `Runtime` 的请求现在携带 context，运行循环接收请求后、开始求值前再次检查取消；对已取消的请求返回取消错误并跳过求值。`TestRuntimeSkipsCanceledRequest` 向运行循环提交已取消请求，确认定义未产生且运行时仍可使用；重复十次通过，`make fmt`、`GOCACHE=/tmp/phi-go-cache make test`、`GOCACHE=/tmp/phi-go-cache make check` 均退出 0。

该检查只收窄求值开始前的竞态；取消仍可能发生在检查之后，运行中的 Glojure 表达式仍不能被中断，现有运行时取消后仍可能产生副作用。不得将此修复作为生产隔离或模型 eval 可用的依据。

补充：运行循环和调用方现在使用无缓冲启动握手。调用方在握手前取消时无需停止整个运行时，运行循环也不会开始该请求；握手完成后取消仍按原行为弃用运行时，因为求值可能已经开始。`TestRuntimeCancelBeforeStart` 对交接后、启动前的取消进行确定性验证；与已有取消测试一起执行 `go test -race` 重复十次通过，`make fmt`、`GOCACHE=/tmp/phi-go-cache make test` 和 `GOCACHE=/tmp/phi-go-cache make check` 均退出 0。握手不终止已经开始的表达式，进程隔离仍未实现。

修正：原先运行循环丢弃已取消请求时仍尝试向可能已离开的调用方发送响应，可能永久阻塞。现直接丢弃该请求；回归测试使用无人接收的响应通道并确认后续求值完成。相关取消测试执行 `go test -race` 重复十次通过，`make fmt`、`GOCACHE=/tmp/phi-go-cache make test` 和 `GOCACHE=/tmp/phi-go-cache make check` 均退出 0。

## 17. 内部 worker 协议雏形

`internal/lisp/worker.go` 提供连续求值的流协议，返回值、输出和错误；`worker_process.go` 管理调用方提供的进程，串行请求，并在请求取消时终止、等待进程并丢弃该实例。测试验证连续定义、错误后继续求值、取消后直接子进程已回收及实例不可再用；`go test -race` 重复五次、`make fmt`、`GOCACHE=/tmp/phi-go-cache make test`、`GOCACHE=/tmp/phi-go-cache make check` 均退出 0。

这是尚未接入入口的内部雏形，调用方必须提供隔离配置；本身不创建命名空间、不验证后代退出，也没有权限桥接或对恶意协议输入的完整保护。测试中的 worker 是普通子进程，不能据此宣称模型 eval 安全或已解决整棵进程树的取消。下一步须将启动限定为经过验证的 fail-closed 隔离机制，验证隔离失败、后代和并发取消，再接入调用方；不可把已有外部副作用称为已回滚。

补充：原 `Close` 等待求值互斥锁，卡住的求值会让关闭永久等待；现通过独立的一次性停止路径并发终止和回收直接子进程。`TestWorkerCloseDuringEvaluation` 等待子进程确认收到请求后并发关闭，验证关闭及求值返回；相关测试 `go test -race` 重复五次，`make fmt`、`GOCACHE=/tmp/phi-go-cache make test`、`GOCACHE=/tmp/phi-go-cache make check` 均通过。该修复仍不保证后代终止或受控隔离。

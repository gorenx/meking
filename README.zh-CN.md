# Meking

[English](README.md) | 简体中文

**面向 AI Agent 的本地知识记忆服务，以自然回忆驱动记忆激活。**

Meking 将对话和文档保存为带版本的实体、关系和主张，并保留指向原始证据的关联。Agent 可以通过 MCP 保存和检索记忆；内置 Web UI 用于查看知识图谱、管理文档和提问。

记忆激活模型根据对话中自然发生的回忆及其证据，更新记忆稳定性与难度，并估计激活度随时间的变化。每次受理的观测都会保存，使状态变化能够重放和验证。

Meking 使用 Go 编写，在本地运行，以 SQLite 存储数据。文本生成和向量嵌入模型通过可配置的 OpenAI 兼容 API 接入。知识保存在你的机器上；发送给模型的内容由所配置的模型服务商处理。

## 主要能力

- **为 Agent 提供持久记忆。** 保存有序消息和 Agent 抽取的知识，检索匹配知识及其原始证据。
- **通过自然回忆追踪记忆状态。** 根据当前知识核对实际回忆表达，记录支持判断的证据，并在观测可评分时更新记忆状态。
- **追踪知识变化。** 保持稳定的对象身份和明确的版本，保留可供审阅与解决的冲突候选。
- **从文档构建知识。** 导入文本，抽取实体、关系及可选的主张。接入 Analysis Sidecar 后，可处理 PDF、DOCX、PPTX、XLSX、HTML，并支持按句子处理。
- **围绕知识提问。** 支持 Basic、Local、Global 和 DRIFT 查询，提供流式回答和引用，并可在 Web UI 中浏览实体、社区及生成的报告。
- **划分用户与会话。** 通过 Root Zone 和 Child Zone 组织知识，将 Agent 记忆绑定到用户与会话。

Meking 借鉴了 GraphRAG 在图谱检索、社区发现和报告生成方面的思路，并维护独立的知识模型与处理生命周期。

## 由自然回忆驱动的记忆激活

对话能够提供回忆成功、困难或失败的证据。Meking 为这些观测提供明确的评估与存储流程，并将其关联到知识对象的当前版本。

记忆激活模型借鉴 FSRS 的记忆状态计算方法：

| 状态 | 含义 |
| --- | --- |
| 稳定性 S | 记忆保持的时间尺度，单位为天。 |
| 难度 D | 通过回忆提高稳定性的难易程度。 |
| 激活度 R | 根据记忆状态和经过的时间，估计某一时刻的可回忆程度。 |

宿主应用（Host）固定观测身份、目标、时间和原始材料，包括回忆表达当时可见的内容。评估 Agent 按所提供的协议判断回忆表现，并引用支持判断的证据。Meking 校验提交，对可评分的观测只应用一次，在同一事务中保存观测和状态更新。

**观测质量决定状态更新是否有依据。** 检索命中、引用、确认或反复提及不会自动强化记忆。评估协议要求检查答案是否已经暴露，以及证据是否足以支持判断。回忆成功但难度不明时，结果为无法评分，不使用默认等级；无法评分的观测仍会保存，但不更新记忆状态。

观测历史保留原始评估、时间、协议和模型身份。重试不会重复应用同一观测；启动时使用原模型参数重放历史输入，核验已保存状态的一致性。

**当前范围：** 已实现观测评估契约、状态计算、持久化和重放验证。激活度尚未参与检索排序，Meking 也不会自动安排复习。模型提供的是记忆状态估计，对 Agent 召回质量的改善仍需通过真实场景评测验证。

## 快速开始

需要 Go 1.26+、Node.js 22.12+ 和 npm，以及可用的文本生成与向量嵌入模型。

```bash
git clone https://github.com/gorenx/meking.git
cd meking
make frontend-sync
make init ../meking-data
```

初始化会在 `../meking-data` 中创建 `settings.yaml`、`.env` 和提示词文件。

1. 在 `../meking-data/.env` 中设置 `MEKING_API_KEY`。
2. 检查 `../meking-data/settings.yaml` 中的文本生成与向量嵌入模型。默认分别为 `gpt-4.1` 和 `text-embedding-3-large`；可以为两者分别配置服务商、API 地址和凭据引用。
3. 启动服务：

```bash
make start START_ROOT=../meking-data
```

打开 **http://127.0.0.1:8080** 访问 Web UI。创建 Zone、上传文本文件，查看处理进度后即可查询生成的知识。

默认的文本输入与 token 分块配置不需要 Python Sidecar。配置在启动时加载，修改设置或凭据后需要重启服务。

## 接入 Agent

运行中的 HTTP 服务提供 Streamable HTTP MCP 端点：

```text
http://127.0.0.1:8080/api/v1/mcp
```

在 MCP 客户端中配置此地址，即可使用以下工具：

| 能力 | 工具 |
| --- | --- |
| 保存与检索记忆 | `add_memory`、`search_memory` |
| 审阅冲突知识 | 实体、关系和主张对应的 `list_*_conflicts`、`get_*_conflict`、`resolve_*_conflict` |
| 删除知识 | `delete_entity`、`delete_relation`、`delete_claim` |
| 评估自然回忆 | `get_recall_evaluation_protocol`、`submit_recall_observation` |

记忆工具接收 `user_id` 和 UUID 格式的 `session_id`。请为同一用户和对话保持这些标识稳定。首次使用时，Meking 会创建对应的 User Root Zone 和 Session Child Zone。

回忆提交需要额外的 Host 集成：Host 在模型可见的工具参数之外绑定观测身份、目标、时间和证据；Agent 只提供评估结果和已有证据的引用。接入前请先读取协议工具返回的 Schema。

构建后的可执行文件也支持 STDIO 传输：

```bash
./bin/meking mcp --root ../meking-data
```

## 可选的文档分析

处理富文档或按句子处理内容时，先安装 Python 3.12 和 `uv`，再准备 Sidecar：

```bash
make sidecar-sync
make sidecar-resources
```

在 Project 设置中选择对应的输入或分块模式。`make start` 会向服务传入仓库内的 Sidecar 可执行文件。依赖包和语言资源需要提前准备。

## 构建与测试

从仓库根目录执行以下命令。完整构建需要先准备前端依赖和上述 Sidecar 运行环境。

```bash
make build          # 构建 Go 可执行文件、Web UI，并准备 Sidecar 环境
make test           # 运行 Go 测试
make quality        # 运行 Go 测试、竞态检测和 vet
make release-check  # 额外验证 Sidecar 测试与前端制品
```

Go 可执行文件输出到 `bin/meking`。

## 部署说明

- Project 数据持久保存在本地 SQLite 和 Project 文件系统中。备份时请保留整个 Project 目录及其配置。
- 服务没有内置认证。请保持默认的本机回环地址监听；需要通过网络访问时，应置于带认证的网关之后。User 和 Session 标识不作为认证凭据。
- 文本生成与向量嵌入请求使用所配置的服务商凭据，可能产生服务商费用。

# Eino-RAG

基于 [Eino 框架](https://github.com/cloudwego/eino) 的 RAG（Retrieval Augmented Generation）和聊天系统演示项目。

## 项目简介

本项目展示了如何使用 Eino 框架构建两种常见的大模型应用：

1. **RAG 系统** (`rag/`) - 知识库问答系统，支持文档注入、向量检索和智能问答
2. **聊天系统** (`chat/`) - 基础对话系统，支持流式输出和对话历史管理

## 特性

### RAG 系统特性
- 📚 **知识库管理**: 自动加载和处理文本文档
- 🔍 **向量检索**: 基于 Qdrant 向量数据库的语义检索
- 🤖 **智能问答**: 结合检索结果的上下文感知回答
- 📊 **文档分片**: 智能文档切分，支持重叠处理
- 🔄 **批处理**: 高效的向量化批处理

### 聊天系统特性
- 💬 **交互式对话**: 命令行交互式聊天界面
- 🌊 **流式输出**: 实时流式回复体验
- 📝 **历史管理**: 完整的对话历史记录
- 🎯 **模板化**: 灵活的提示词模板系统

## 环境要求

- Go 1.21+
- Docker (用于运行 Qdrant 向量数据库)
- 有效的 OpenAI API 密钥或兼容的 API 服务

## 安装和设置

### 1. 克隆项目

```bash
git clone https://github.com/chegangan/eino-rag.git
cd eino-rag
```

### 2. 安装依赖

```bash
go mod tidy
```

### 3. 配置 API 密钥

在运行前需要配置你的 API 密钥：

**RAG 系统配置** (`rag/main.go`):
```go
const (
    BaseURL      = "https://api.siliconflow.cn/v1"  // 你的 API 基础 URL
    OpenAIAPIKey = "your-api-key-here"              // 替换为你的 API Key
    EmbeddingModel = "BAAI/bge-m3"                  // 嵌入模型
    LLMModel       = "Qwen/Qwen3-8B"                // 语言模型
)
```

**聊天系统配置** (`chat/openai.go`):
```go
var (
    OPENAI_BASE_URL   = "https://api.siliconflow.cn/v1"
    OPENAI_API_KEY    = "your-api-key-here"
    OPENAI_MODEL_NAME = "Qwen/Qwen3-8B"
)
```

### 4. 启动 Qdrant 向量数据库 (仅 RAG 系统需要)

```bash
cd rag
chmod +x qdrant.sh
./qdrant.sh
```

或者手动运行：
```bash
docker run -d \
  --name qdrant \
  --rm \
  -p 6333:6333 \
  -p 6334:6334 \
  -v $(pwd)/qdrant_storage:/qdrant/storage:z \
  qdrant/qdrant
```

## 使用说明

### RAG 系统

1. 确保 Qdrant 数据库正在运行
2. 运行 RAG 系统：

```bash
cd rag
go run main.go
```

**功能说明**:
- 程序启动时会自动检查并创建示例知识库文件 `knowledge.txt`
- 自动将知识库文档分片并向量化存储到 Qdrant
- 执行预设问题的智能问答演示
- 显示检索到的相关文档片段和最终答案

### 聊天系统

```bash
cd chat
go run .
```

**使用方式**:
- 启动后在命令行输入消息与 AI 对话
- 支持多轮对话，系统会保持对话历史
- 输入 `Ctrl+C` 退出程序

## 配置说明

### RAG 系统配置项

| 配置项 | 默认值 | 说明 |
|--------|--------|------|
| `QdrantHost` | `localhost` | Qdrant 服务器地址 |
| `QdrantPort` | `6334` | Qdrant gRPC 端口 |
| `CollectionName` | `eino_best_practice_kb` | 向量集合名称 |
| `VectorDim` | `1024` | 向量维度 |
| `ChunkSize` | `500` | 文档分片大小 |
| `ChunkOverlap` | `100` | 分片重叠字符数 |
| `TopK` | `5` | 检索返回的文档数量 |
| `EmbeddingBatchSize` | `32` | 向量化批处理大小 |

### 聊天系统配置项

聊天系统配置相对简单，主要需要配置 API 相关信息。

## 项目结构

```
eino-rag/
├── README.md                 # 项目说明文档
├── go.mod                    # Go 模块定义
├── go.sum                    # 依赖版本锁定
├── .gitignore               # Git 忽略文件
├── chat/                    # 聊天系统
│   ├── main.go             # 主程序入口
│   ├── openai.go           # OpenAI 客户端配置
│   ├── template.go         # 消息模板处理
│   ├── generate.go         # 消息生成逻辑
│   └── steam.go            # 流式输出处理
└── rag/                     # RAG 系统
    ├── main.go             # 主程序入口
    ├── knowledge.txt       # 知识库文件（自动生成）
    ├── qdrant.sh          # Qdrant 启动脚本
    └── qdrant_storage/    # Qdrant 数据存储目录
```

## 技术栈

- **[Eino 框架](https://github.com/cloudwego/eino)**: 字节跳动开源的大模型应用开发框架
- **[Qdrant](https://qdrant.tech/)**: 高性能向量数据库
- **OpenAI API**: 大语言模型和嵌入模型服务
- **Go**: 主要编程语言

## 常见问题

### Q: Qdrant 连接失败怎么办？
A: 确保 Docker 正在运行，并且 Qdrant 容器已启动。检查端口 6334 是否被占用。

### Q: API 调用失败怎么办？
A: 检查 API 密钥是否正确配置，网络连接是否正常，API 服务是否可用。

### Q: 知识库文件在哪里？
A: 知识库文件位于 `rag/knowledge.txt`，如果不存在会自动创建示例文件。你可以替换为自己的知识库内容。

### Q: 如何自定义知识库？
A: 直接编辑 `rag/knowledge.txt` 文件，添加你的知识库内容。程序会自动处理文档分片和向量化。

### Q: 支持其他语言模型吗？
A: 支持。只需修改配置中的模型名称和 API 地址即可使用其他兼容 OpenAI API 的服务。

## 贡献

欢迎提交 Issue 和 Pull Request 来改进这个项目。

## 许可证

本项目采用 MIT 许可证。详见 LICENSE 文件。

## 相关链接

- [Eino 框架官方文档](https://github.com/cloudwego/eino)
- [Qdrant 向量数据库](https://qdrant.tech/)
- [字节跳动 CloudWeGo](https://www.cloudwego.io/)
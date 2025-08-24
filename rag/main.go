package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	// Eino 核心及扩展组件
	"github.com/cloudwego/eino-ext/components/document/loader/file"
	"github.com/cloudwego/eino-ext/components/document/transformer/splitter/recursive"
	"github.com/cloudwego/eino-ext/components/embedding/openai"
	eino_openai "github.com/cloudwego/eino-ext/components/model/openai"
	"github.com/cloudwego/eino/callbacks"
	"github.com/cloudwego/eino/components/document"
	"github.com/cloudwego/eino/components/embedding"
	"github.com/cloudwego/eino/components/indexer"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/components/prompt"
	"github.com/cloudwego/eino/components/retriever"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"

	"github.com/google/uuid"
	"github.com/qdrant/go-client/qdrant"
)

// ================== 1. 配置中心 ==================
const (
	QdrantHost = "localhost"
	QdrantPort = 6334

	CollectionName    = "eino_best_practice_kb"
	VectorDim         = 1024
	DocMetaDataVector = "embedding_vector" // 用于在 Document.MetaData 中存储向量的键
	QdrantPayloadKey  = "content"          // 用于在 Qdrant Payload 中存储文档内容的键

	BaseURL        = "https://api.siliconflow.cn/v1"                       // OpenAI API 基础 URL
	OpenAIAPIKey   = "sk-sgqsukbjugibutigmmdwpeyzgkmazmajqvohpkblytwfigxc" // 务必替换为你的 OpenAI API Key
	EmbeddingModel = "BAAI/bge-m3"
	LLMModel       = "Qwen/Qwen3-8B"
	Timeout        = 60 * time.Second

	KnowledgeFilePath  = "knowledge.txt"
	ChunkSize          = 500
	ChunkOverlap       = 100
	TopK               = 5  // 检索时返回的文档数量
	EmbeddingBatchSize = 32 // Embedding API允许的最大批处理大小
)

var (
	ChunkSeparators = []string{"\n\n", "\n", "。", "！", "？", " "}
)

// ================== 2. 自定义组件 ==================

// --- 2.1 Qdrant Indexer (已重构) ---
// QdrantIndexer 现在只负责存储已经包含向量的文档
type QdrantIndexer struct {
	client     *qdrant.Client
	collection string
}

// NewQdrantIndexer 不再需要 embedder
func NewQdrantIndexer(client *qdrant.Client, collection string) *QdrantIndexer {
	return &QdrantIndexer{
		client:     client,
		collection: collection,
	}
}

// Store 方法不再执行 embedding，而是从 doc.MetaData 中获取向量
func (q *QdrantIndexer) Store(ctx context.Context, docs []*schema.Document, opts ...indexer.Option) ([]string, error) {
	var points []*qdrant.PointStruct
	storedIDs := make([]string, len(docs))

	for i, doc := range docs {
		if doc.ID == "" {
			doc.ID = uuid.NewString()
		}
		storedIDs[i] = doc.ID

		// 从 MetaData 获取向量
		vectorVal, ok := doc.MetaData[DocMetaDataVector]
		if !ok {
			return nil, fmt.Errorf("document with ID %s does not have an embedding vector in MetaData", doc.ID)
		}
		vector64, ok := vectorVal.([]float64)
		if !ok {
			return nil, fmt.Errorf("embedding vector for doc ID %s is not of type []float64", doc.ID)
		}

		payloadMap := make(map[string]interface{})
		payloadMap[QdrantPayloadKey] = doc.Content
		payload := qdrant.NewValueMap(payloadMap)

		// 类型转换
		vector32 := make([]float32, len(vector64))
		for j, v := range vector64 {
			vector32[j] = float32(v)
		}

		points = append(points, &qdrant.PointStruct{
			Id:      qdrant.NewIDUUID(doc.ID),
			Vectors: qdrant.NewVectors(vector32...),
			Payload: payload,
		})
	}

	if len(points) == 0 {
		log.Println("⚠️ 没有有效的点需要存储")
		return storedIDs, nil
	}

	_, err := q.client.Upsert(ctx, &qdrant.UpsertPoints{
		CollectionName: q.collection,
		Points:         points,
	})
	if err != nil {
		return nil, fmt.Errorf("upserting points to Qdrant: %w", err)
	}

	return storedIDs, nil
}

// --- 2.2 Qdrant Retriever (保持不变) ---
type QdrantRetriever struct {
	client     *qdrant.Client
	collection string
	embedder   embedding.Embedder
	topK       uint64
}

func NewQdrantRetriever(client *qdrant.Client, collection string, embedder embedding.Embedder, topK uint64) *QdrantRetriever {
	return &QdrantRetriever{
		client:     client,
		collection: collection,
		embedder:   embedder,
		topK:       topK,
	}
}

func (q *QdrantRetriever) Retrieve(ctx context.Context, query string, opts ...retriever.Option) ([]*schema.Document, error) {
	options := &retriever.Options{
		Embedding: q.embedder,
		TopK:      new(int),
	}
	*options.TopK = int(q.topK)
	options = retriever.GetCommonOptions(options, opts...)

	if options.Embedding == nil {
		return nil, fmt.Errorf("retriever requires an embedder")
	}

	queryVectors64, err := options.Embedding.EmbedStrings(ctx, []string{query})
	if err != nil {
		return nil, fmt.Errorf("embedding query: %w", err)
	}
	if len(queryVectors64) == 0 {
		return nil, fmt.Errorf("embedding query returned no vectors")
	}

	queryVector32 := make([]float32, len(queryVectors64[0]))
	for i, v := range queryVectors64[0] {
		queryVector32[i] = float32(v)
	}

	limit := uint64(*options.TopK)
	searchResult, err := q.client.Query(ctx, &qdrant.QueryPoints{
		CollectionName: q.collection,
		Query:          qdrant.NewQuery(queryVector32...),
		Limit:          &limit,
		WithPayload:    qdrant.NewWithPayload(true),
	})
	if err != nil {
		return nil, fmt.Errorf("searching Qdrant: %w", err)
	}

	var docs []*schema.Document
	for _, hit := range searchResult {
		contentValue, ok := hit.Payload[QdrantPayloadKey]
		if !ok {
			continue
		}
		content := contentValue.GetStringValue()
		metaData := map[string]interface{}{
			DocMetaDataVector: float64(hit.Score),
		}
		docs = append(docs, &schema.Document{
			ID:       hit.GetId().GetUuid(),
			Content:  content,
			MetaData: metaData,
		})
		fmt.Println("Retrieved document:", content)
	}
	return docs, nil
}

// --- 2.3 Embedding Transformer (新增) ---
// EmbeddingTransformer 是一个文档转换器，用于为文档生成向量并存入MetaData
type EmbeddingTransformer struct {
	embedder embedding.Embedder
}

func NewEmbeddingTransformer(embedder embedding.Embedder) *EmbeddingTransformer {
	return &EmbeddingTransformer{
		embedder: embedder,
	}
}

// Transform 实现了 document.Transformer 接口
func (t *EmbeddingTransformer) Transform(ctx context.Context, src []*schema.Document, opts ...document.TransformerOption) ([]*schema.Document, error) {
	if len(src) == 0 {
		return src, nil
	}

	numDocs := len(src)
	allVectors := make([][]float64, 0, numDocs)

	log.Printf("准备为 %d 个文档块进行向量化（批处理大小：%d）...", numDocs, EmbeddingBatchSize)

	// 使用循环按批次处理文档
	for i := 0; i < numDocs; i += EmbeddingBatchSize {
		end := i + EmbeddingBatchSize
		if end > numDocs {
			end = numDocs
		}

		batchDocs := src[i:end]
		textsToEmbed := make([]string, len(batchDocs))
		for j, doc := range batchDocs {
			textsToEmbed[j] = doc.Content
		}

		log.Printf("正在处理批次 %d -> %d", i, end-1)

		// 对当前批次进行 Embedding
		vectors, err := t.embedder.EmbedStrings(ctx, textsToEmbed)
		if err != nil {
			// 在错误信息中加入批次信息，方便调试
			return nil, fmt.Errorf("embedding documents in transformer (batch %d-%d): %w", i, end-1, err)
		}

		if len(vectors) != len(batchDocs) {
			return nil, fmt.Errorf("批次向量数量 (%d) 与文档数量 (%d) 不匹配", len(vectors), len(batchDocs))
		}

		allVectors = append(allVectors, vectors...)
	}

	if len(allVectors) != numDocs {
		return nil, fmt.Errorf("最终向量总数 (%d) 与文档总数 (%d) 不匹配", len(allVectors), numDocs)
	}

	// 将所有生成好的向量附加到原始文档的 MetaData 中
	for i, doc := range src {
		if doc.MetaData == nil {
			doc.MetaData = make(map[string]interface{})
		}
		doc.MetaData[DocMetaDataVector] = allVectors[i]
	}

	log.Println("✅ 所有文档块向量化完成")
	return src, nil
}

// ================== 3. 回调处理器 (保持不变) ==================
type loggerCallbacks struct{}

func (l *loggerCallbacks) OnStart(ctx context.Context, info *callbacks.RunInfo, input callbacks.CallbackInput) context.Context {
	log.Printf("[CALLBACK] ▶️  START: %s (%s) | Component: %s", info.Name, info.Type, info.Component)
	return ctx
}
func (l *loggerCallbacks) OnEnd(ctx context.Context, info *callbacks.RunInfo, output callbacks.CallbackOutput) context.Context {
	log.Printf("[CALLBACK] ✅  END: %s (%s) | Component: %s", info.Name, info.Type, info.Component)
	return ctx
}
func (l *loggerCallbacks) OnError(ctx context.Context, info *callbacks.RunInfo, err error) context.Context {
	log.Printf("[CALLBACK] ❌ ERROR: %s (%s) | Component: %s | Error: %v", info.Name, info.Type, info.Component, err)
	return ctx
}
func (l *loggerCallbacks) OnStartWithStreamInput(ctx context.Context, info *callbacks.RunInfo, input *schema.StreamReader[callbacks.CallbackInput]) context.Context {
	return ctx
}
func (l *loggerCallbacks) OnEndWithStreamOutput(ctx context.Context, info *callbacks.RunInfo, output *schema.StreamReader[callbacks.CallbackOutput]) context.Context {
	return ctx
}

// ================== 4. 核心业务逻辑 (已重构) ==================

// ingestKnowledge 负责将指定文件注入知识库
func ingestKnowledge(ctx context.Context, qdrantClient *qdrant.Client, embedder embedding.Embedder, filePath string) error {
	log.Println("\n--- 知识注入流程开始 ---")

	// 1. 初始化所有需要的组件
	loader, err := file.NewFileLoader(ctx, &file.FileLoaderConfig{UseNameAsID: false})
	if err != nil {
		return fmt.Errorf("创建 FileLoader 失败: %v", err)
	}

	splitter, err := recursive.NewSplitter(ctx, &recursive.Config{
		ChunkSize:   ChunkSize,
		OverlapSize: ChunkOverlap,
		Separators:  ChunkSeparators,
		IDGenerator: func(ctx context.Context, originalID string, splitIndex int) string {
			return uuid.NewString()
		},
	})
	if err != nil {
		return fmt.Errorf("创建 RecursiveSplitter 失败: %v", err)
	}

	// 新增的 EmbeddingTransformer
	embeddingTransformer := NewEmbeddingTransformer(embedder)

	// 重构后的 QdrantIndexer
	indexerComponent := NewQdrantIndexer(qdrantClient, CollectionName)

	// 2. 构建并编排注入链
	ingestionChain := compose.NewChain[document.Source, []string]()
	ingestionChain.AppendLoader(loader)
	ingestionChain.AppendDocumentTransformer(splitter)
	ingestionChain.AppendDocumentTransformer(embeddingTransformer) // 在 Indexer 之前进行 embedding
	ingestionChain.AppendIndexer(indexerComponent)

	runnable, err := ingestionChain.Compile(ctx)
	if err != nil {
		return fmt.Errorf("编译 Ingestion Chain 失败: %v", err)
	}

	// 3. 执行链
	log.Printf("📚 正在从 %s 加载、分割、向量化和索引知识...", filePath)
	_, err = runnable.Invoke(ctx, document.Source{URI: filePath})
	if err != nil {
		return fmt.Errorf("执行 Ingestion Chain 失败: %v", err)
	}

	log.Println("--- ✅ 知识注入流程成功 ---")
	return nil
}

// answerQuery 负责根据用户问题，从知识库检索并生成答案
func answerQuery(ctx context.Context, llm model.ToolCallingChatModel, qdrantClient *qdrant.Client, embedder embedding.Embedder, userQuery string) (string, error) {
	log.Println("\n--- RAG 问答流程开始 ---")

	// 1. 初始化 Retriever
	ragRetriever := NewQdrantRetriever(qdrantClient, CollectionName, embedder, uint64(TopK))

	// 2. 构建 RAG 图
	ragGraph := compose.NewGraph[map[string]interface{}, *schema.Message]()

	// 2.1 Retriever 节点: 输入 "query" 字符串，输出 map{"documents": ...}
	ragGraph.AddRetrieverNode("retriever", ragRetriever,
		compose.WithInputKey("query"),
		compose.WithOutputKey("documents"),
	)

	// 2.2 准备提示词输入节点 (Lambda): 执行自定义的数据格式化逻辑，仍然是必需的
	preparePromptInputLambda := compose.InvokableLambda(
		func(ctx context.Context, input map[string]interface{}) (map[string]interface{}, error) {
			docsVal, ok := input["documents"]
			if !ok {
				return nil, fmt.Errorf("'documents' key not found in input for prompt preparation")
			}
			docs, ok := docsVal.([]*schema.Document)
			if !ok {
				return nil, fmt.Errorf("value for 'documents' is not of type []*schema.Document")
			}
			queryVal, ok := input["query"]
			if !ok {
				return nil, fmt.Errorf("'query' key not found in input for prompt preparation")
			}
			var b strings.Builder
			if len(docs) == 0 {
				b.WriteString("没有找到相关上下文。")
			} else {
				b.WriteString("请参考以下上下文信息：\n\n")
				for i, doc := range docs {
					scoreVal, _ := doc.MetaData[DocMetaDataVector]
					score, _ := scoreVal.(float64)
					b.WriteString(fmt.Sprintf("--- 上下文 %d (相似度: %.4f) ---\n%s\n\n", i+1, score, doc.Content))
				}
			}
			return map[string]interface{}{
				"context_str": b.String(),
				"query":       queryVal,
			}, nil
		},
	)
	ragGraph.AddLambdaNode("prepare_prompt_input", preparePromptInputLambda)

	// 2.3 提示词模板节点
	template := prompt.FromMessages(schema.FString,
		schema.SystemMessage("你是一个智能问答助手。请根据下面提供的上下文来回答问题。如果上下文中没有相关信息，就明确说你不知道，不要编造答案。"),
		schema.UserMessage("上下文：\n{context_str}\n---\n问题：{query}"),
	)
	ragGraph.AddChatTemplateNode("prompt_template", template)

	// 2.4 LLM 调用节点
	ragGraph.AddChatModelNode("llm", llm)

	// 2.5 连接所有节点 (保持“扇入”结构)
	ragGraph.AddEdge(compose.START, "retriever")
	ragGraph.AddEdge(compose.START, "prepare_prompt_input")
	ragGraph.AddEdge("retriever", "prepare_prompt_input")
	ragGraph.AddEdge("prepare_prompt_input", "prompt_template")
	ragGraph.AddEdge("prompt_template", "llm")
	ragGraph.AddEdge("llm", compose.END)

	// 3. 编译图
	runnable, err := ragGraph.Compile(ctx, compose.WithNodeTriggerMode(compose.AllPredecessor))
	if err != nil {
		return "", fmt.Errorf("编译 RAG Graph 失败: %v", err)
	}

	log.Printf("🔍 正在查询: %s", userQuery)
	input := map[string]interface{}{"query": userQuery}
	response, err := runnable.Invoke(ctx, input)
	if err != nil {
		return "", fmt.Errorf("执行 RAG Graph 失败: %v", err)
	}

	log.Printf("✅ RAG 回答: %s", response.Content)
	log.Println("--- RAG 问答流程结束 ---")
	return response.Content, nil
}

// ================== 5. 设置与主函数 (已重构) ==================

// setupComponents 负责初始化所有外部依赖的客户端和组件
func setupComponents(ctx context.Context) (model.ToolCallingChatModel, embedding.Embedder, *qdrant.Client, error) {
	// 初始化 LLM
	llm, err := eino_openai.NewChatModel(ctx, &eino_openai.ChatModelConfig{
		BaseURL: BaseURL,
		APIKey:  OpenAIAPIKey,
		Model:   LLMModel,
		Timeout: Timeout,
	})
	if err != nil {
		return nil, nil, nil, fmt.Errorf("❌ 初始化 LLM 失败: %v", err)
	}

	// 初始化 Embedder
	embedder, err := openai.NewEmbedder(ctx, &openai.EmbeddingConfig{
		BaseURL: BaseURL,
		APIKey:  OpenAIAPIKey,
		Model:   EmbeddingModel,
		Timeout: Timeout,
	})
	if err != nil {
		return nil, nil, nil, fmt.Errorf("❌ 初始化 Embedder 失败: %v", err)
	}

	// 初始化 Qdrant 客户端
	qdrantClient, err := qdrant.NewClient(&qdrant.Config{
		Host: QdrantHost,
		Port: QdrantPort,
	})
	if err != nil {
		return nil, nil, nil, fmt.Errorf("❌ 连接 Qdrant 失败: %v", err)
	}

	// 检查并创建 Qdrant 集合
	exists, err := qdrantClient.CollectionExists(ctx, CollectionName)
	if err != nil {
		qdrantClient.Close()
		return nil, nil, nil, fmt.Errorf("❌ 检查集合是否存在时出错: %v", err)
	}

	if !exists {
		log.Printf("📁 集合 '%s' 不存在，正在创建...", CollectionName)
		err = qdrantClient.CreateCollection(ctx, &qdrant.CreateCollection{
			CollectionName: CollectionName,
			VectorsConfig: qdrant.NewVectorsConfig(&qdrant.VectorParams{
				Size:     uint64(VectorDim),
				Distance: qdrant.Distance_Cosine,
			}),
		})
		if err != nil {
			qdrantClient.Close()
			return nil, nil, nil, fmt.Errorf("❌ 创建集合失败: %v", err)
		}
		log.Printf("✅ 集合 '%s' 创建成功", CollectionName)
	} else {
		log.Printf("🔁 集合 '%s' 已存在", CollectionName)
	}

	return llm, embedder, qdrantClient, nil
}

// prepareKnowledgeFile 检查知识库文件是否存在，如果不存在则创建一个示例文件
func prepareKnowledgeFile() {
	if _, err := os.Stat(KnowledgeFilePath); os.IsNotExist(err) {
		log.Printf("📜 未找到知识库文件 %s，正在创建一个示例文件...", KnowledgeFilePath)
		content := "Eino 是一个开源的大模型应用开发框架。\n它由字节跳动 CloudWeGo 团队开发，旨在简化和加速大模型应用的构建。\nEino 的核心是组件化和可编排性，提供了丰富的内置组件和灵活的 Chain/Graph 编排能力。"
		_ = os.WriteFile(KnowledgeFilePath, []byte(content), 0644)
	}
}

func main() {
	ctx := context.Background()
	callbacks.AppendGlobalHandlers(&loggerCallbacks{})
	prepareKnowledgeFile()

	// 1. 初始化所有组件
	llm, embedder, qdrantClient, err := setupComponents(ctx)
	if err != nil {
		log.Fatalf("%s", err.Error())
	}
	defer qdrantClient.Close()

	// 2. 执行知识注入流程
	err = ingestKnowledge(ctx, qdrantClient, embedder, KnowledgeFilePath)
	if err != nil {
		log.Fatalf("知识注入失败: %v", err)
	}

	// 3. 执行问答流程
	userQuestion := "Eino 框架是什么？它有什么特点？"
	_, err = answerQuery(ctx, llm, qdrantClient, embedder, userQuestion)
	if err != nil {
		log.Fatalf("问答查询失败: %v", err)
	}
}

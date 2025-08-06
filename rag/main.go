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

	CollectionName = "eino_best_practice_kb"
	VectorDim      = 1536

	OpenAIAPIKey   = "<YOUR_OPENAI_API_KEY>"
	EmbeddingModel = "text-embedding-ada-002"
	LLMModel       = "gpt-3.5-turbo"
	Timeout        = 60 * time.Second

	KnowledgeFilePath = "knowledge.txt"
	ChunkSize         = 500
	ChunkOverlap      = 100
	TopK              = 3
)

var (
	ChunkSeparators = []string{"\n\n", "\n", "。", "！", "？", " "}
)

// ================== 2. 自定义 Qdrant 组件 (已修正) ==================

type QdrantIndexer struct {
	client     *qdrant.Client
	collection string
	embedder   embedding.Embedder
}

func NewQdrantIndexer(client *qdrant.Client, collection string, embedder embedding.Embedder) *QdrantIndexer {
	return &QdrantIndexer{
		client:     client,
		collection: collection,
		embedder:   embedder,
	}
}

func (q *QdrantIndexer) Store(ctx context.Context, docs []*schema.Document, opts ...indexer.Option) ([]string, error) {
	options := &indexer.Options{Embedding: q.embedder}
	options = indexer.GetCommonOptions(options, opts...)

	if options.Embedding == nil {
		return nil, fmt.Errorf("indexer requires an embedder")
	}

	var points []*qdrant.PointStruct
	textsToEmbed := make([]string, len(docs))
	storedIDs := make([]string, len(docs))

	for i, doc := range docs {
		if doc.ID == "" {
			doc.ID = uuid.NewString()
		}
		storedIDs[i] = doc.ID
		textsToEmbed[i] = doc.Content
	}

	vectors64, err := options.Embedding.EmbedStrings(ctx, textsToEmbed)
	if err != nil {
		return nil, fmt.Errorf("embedding documents: %w", err)
	}

	for i, doc := range docs {
		payloadMap := doc.MetaData
		if payloadMap == nil {
			payloadMap = make(map[string]interface{})
		}
		payloadMap["content"] = doc.Content

		// 【修正】使用 qdrant.NewValueMap 辅助函数
		payload := qdrant.NewValueMap(payloadMap)

		vector32 := make([]float32, len(vectors64[i]))
		for j, v := range vectors64[i] {
			vector32[j] = float32(v)
		}

		points = append(points, &qdrant.PointStruct{
			// 【修正】使用 qdrant的uuid，这里直接用doc的id
			Id: qdrant.NewIDUUID(doc.ID),
			// 【修正】将 slice 传递给 variadic 函数需要使用 '...'
			Vectors: qdrant.NewVectors(vector32...),
			Payload: payload,
		})
	}

	if len(points) == 0 {
		log.Println("⚠️  没有有效的点需要存储")
		return storedIDs, nil
	}

	// 【修正】正确的方法名是 Upsert
	_, err = q.client.Upsert(ctx, &qdrant.UpsertPoints{
		CollectionName: q.collection,
		Points:         points,
	})
	if err != nil {
		return nil, fmt.Errorf("upserting points to Qdrant: %w", err)
	}
	return storedIDs, nil
}

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

	// 【修正】正确的方法名是 Query
	searchResult, err := q.client.Query(ctx, &qdrant.QueryPoints{
		CollectionName: q.collection,
		// 【修正】使用 qdrant.NewQuery 并展开 slice
		Query: qdrant.NewQuery(queryVector32...),
	})
	if err != nil {
		return nil, fmt.Errorf("searching Qdrant: %w", err)
	}

	var docs []*schema.Document
	for _, hit := range searchResult {
		contentValue := hit.GetPayload()["content"]

		content := contentValue.GetStringValue()
		metaData := map[string]interface{}{
			"score": float64(hit.Score),
		}
		docs = append(docs, &schema.Document{
			ID:       hit.GetId().GetUuid(),
			Content:  content,
			MetaData: metaData,
		})
	}
	return docs, nil
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

// ================== 4. 核心业务逻辑 ==================

func setupComponents(ctx context.Context) (model.ToolCallingChatModel, embedding.Embedder, *qdrant.Client, error) {
	if OpenAIAPIKey == "<YOUR_OPENAI_API_KEY>" {
		return nil, nil, nil, fmt.Errorf("❌ 请在代码中设置你的 OpenAI API Key")
	}

	llm, err := eino_openai.NewChatModel(ctx, &eino_openai.ChatModelConfig{
		APIKey:  OpenAIAPIKey,
		Model:   LLMModel,
		Timeout: Timeout,
	})
	if err != nil {
		return nil, nil, nil, fmt.Errorf("❌ 初始化 LLM 失败: %v", err)
	}

	embedder, err := openai.NewEmbedder(ctx, &openai.EmbeddingConfig{
		APIKey:  OpenAIAPIKey,
		Model:   EmbeddingModel,
		Timeout: Timeout,
	})
	if err != nil {
		return nil, nil, nil, fmt.Errorf("❌ 初始化 Embedder 失败: %v", err)
	}

	// 【修正】使用 &qdrant.Config{} 初始化客户端
	qdrantClient, err := qdrant.NewClient(&qdrant.Config{
		Host: QdrantHost,
		Port: QdrantPort,
		// 如果使用 Qdrant Cloud，可以添加 APIKey 和 UseTLS: true
	})
	if err != nil {
		return nil, nil, nil, fmt.Errorf("❌ 连接 Qdrant 失败: %v", err)
	}

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

func runIngestion(ctx context.Context, qdrantClient *qdrant.Client, embedder embedding.Embedder) error {
	log.Println("\n--- 知识注入流程开始 ---")

	loader, err := file.NewFileLoader(ctx, &file.FileLoaderConfig{UseNameAsID: true})
	if err != nil {
		return fmt.Errorf("创建 FileLoader 失败: %v", err)
	}

	splitter, err := recursive.NewSplitter(ctx, &recursive.Config{
		ChunkSize:   ChunkSize,
		OverlapSize: ChunkOverlap,
		Separators:  ChunkSeparators,
	})
	if err != nil {
		return fmt.Errorf("创建 RecursiveSplitter 失败: %v", err)
	}

	indexerComponent := NewQdrantIndexer(qdrantClient, CollectionName, embedder)

	ingestionChain := compose.NewChain[document.Source, []string]()
	ingestionChain.AppendLoader(loader)
	ingestionChain.AppendDocumentTransformer(splitter)
	ingestionChain.AppendIndexer(indexerComponent)

	runnable, err := ingestionChain.Compile(ctx)
	if err != nil {
		return fmt.Errorf("编译 Ingestion Chain 失败: %v", err)
	}

	log.Printf("📚 正在从 %s 加载、分割和索引知识...", KnowledgeFilePath)
	_, err = runnable.Invoke(ctx, document.Source{URI: KnowledgeFilePath})
	if err != nil {
		return fmt.Errorf("执行 Ingestion Chain 失败: %v", err)
	}

	log.Println("--- 知识注入流程结束 ---")
	return nil
}

func runQuery(ctx context.Context, llm model.ToolCallingChatModel, ragRetriever retriever.Retriever, userQuery string) (string, error) {
	log.Println("\n--- RAG 问答流程开始 ---")

	ragGraph := compose.NewGraph[map[string]interface{}, *schema.Message]()

	ragGraph.AddRetrieverNode("retriever", ragRetriever,
		compose.WithInputKey("query"),
		compose.WithOutputKey("documents"),
	)

	formatDocsLambda := compose.InvokableLambda(func(ctx context.Context, docs []*schema.Document) (string, error) {
		if len(docs) == 0 {
			return "没有找到相关上下文。", nil
		}
		var b strings.Builder
		b.WriteString("请参考以下上下文信息：\n\n")
		for i, doc := range docs {
			score := doc.MetaData["score"]
			b.WriteString(fmt.Sprintf("--- 上下文 %d (相似度: %.4f) ---\n%s\n\n", i+1, score, doc.Content))
		}
		return b.String(), nil
	})
	ragGraph.AddLambdaNode("format_docs", formatDocsLambda,
		compose.WithInputKey("documents"),
		compose.WithOutputKey("context_str"),
	)

	template := prompt.FromMessages(schema.FString,
		schema.SystemMessage("你是一个智能问答助手。请根据下面提供的上下文来回答问题。如果上下文中没有相关信息，就明确说你不知道，不要编造答案。"),
		schema.UserMessage("上下文：\n{context_str}\n---\n问题：{query}"),
	)
	ragGraph.AddChatTemplateNode("prompt_template", template)

	ragGraph.AddChatModelNode("llm", llm)

	ragGraph.AddEdge(compose.START, "retriever")
	ragGraph.AddEdge("retriever", "format_docs")
	ragGraph.AddEdge(compose.START, "prompt_template")
	ragGraph.AddEdge("format_docs", "prompt_template")
	ragGraph.AddEdge("prompt_template", "llm")
	ragGraph.AddEdge("llm", compose.END)

	runnable, err := ragGraph.Compile(ctx)
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

// ================== 5. 主函数 ==================
func main() {
	ctx := context.Background()
	callbacks.AppendGlobalHandlers(&loggerCallbacks{})

	if _, err := os.Stat(KnowledgeFilePath); os.IsNotExist(err) {
		log.Printf("📜 未找到知识库文件 %s，正在创建一个示例文件...", KnowledgeFilePath)
		content := "Eino 是一个开源的大模型应用开发框架。\n它由字节跳动 CloudWeGo 团队开发，旨在简化和加速大模型应用的构建。\nEino 的核心是组件化和可编排性，提供了丰富的内置组件和灵活的 Chain/Graph 编排能力。"
		_ = os.WriteFile(KnowledgeFilePath, []byte(content), 0644)
	}

	llm, embedder, qdrantClient, err := setupComponents(ctx)
	if err != nil {
		log.Fatalf(err.Error())
	}
	defer qdrantClient.Close()

	err = runIngestion(ctx, qdrantClient, embedder)
	if err != nil {
		log.Fatalf("知识注入失败: %v", err)
	}

	ragRetriever := NewQdrantRetriever(qdrantClient, CollectionName, embedder, uint64(TopK))
	_, err = runQuery(ctx, llm, ragRetriever, "Eino 框架是什么？它有什么特点？")
	if err != nil {
		log.Fatalf("问答查询失败: %v", err)
	}
}

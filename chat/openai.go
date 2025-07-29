package main

import (
	"context"
	"log"

	"github.com/cloudwego/eino-ext/components/model/openai"
	"github.com/cloudwego/eino/components/model"
)

var (
	OPENAI_BASE_URL   = "https://api.siliconflow.cn/v1"
	OPENAI_API_KEY    = "sk-vlceodtoddldfmvszlpbndqnnxnuoiucesqybnnojfpwbroq"
	OPENAI_MODEL_NAME = "Qwen/Qwen3-8B"
)

func createOpenAIChatModel(ctx context.Context) model.ToolCallingChatModel {
	chatModel, err := openai.NewChatModel(ctx, &openai.ChatModelConfig{
		BaseURL: OPENAI_BASE_URL,
		Model:   OPENAI_MODEL_NAME,
		APIKey:  OPENAI_API_KEY,
	})
	if err != nil {
		log.Fatalf("create openai chat model failed, err=%v", err)
	}
	return chatModel
}

package main

import (
	"context"
	"fmt"

	"github.com/cloudwego/eino-ext/components/model/openai"
	"github.com/cloudwego/eino/components/model"
)

// CreateOpenAIChatModel initializes and returns a configured OpenAI chat model
func createOpenAIChatModel(ctx context.Context) (model.ToolCallingChatModel, error) {
	apiKey := GetAPIKey()
	if apiKey == "" || apiKey == "your_api_key_here" {
		return nil, fmt.Errorf("API key not configured")
	}

	chatModel, err := openai.NewChatModel(ctx, &openai.ChatModelConfig{
		BaseURL: GetBaseURL(),
		Model:   GetModelName(),
		APIKey:  apiKey,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create OpenAI chat model: %v", err)
	}
	return chatModel, nil
}

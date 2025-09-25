package main

import (
	"context"
	"log"

	"github.com/cloudwego/eino/components/prompt"
	"github.com/cloudwego/eino/schema"
)

var chatTemplate = prompt.FromMessages(schema.FString,
	schema.SystemMessage("{system_prompt}"),
	schema.MessagesPlaceholder("history", true),
	schema.UserMessage("{question}"),
)

// 生成消息，支持多轮历史
func buildMessages(history []*schema.Message, question string, systemPrompt string) []*schema.Message {
	variables := map[string]any{
		"system_prompt": systemPrompt,
		"question":      question,
		"history":       history,
	}
	messages, err := chatTemplate.Format(context.Background(), variables)
	if err != nil {
		log.Fatalf("format template failed: %v\n", err)
	}
	return messages
}

// 追加一轮对话到历史
func AppendHistory(history []*schema.Message, userInput, assistantMsg string) []*schema.Message {
	history = append(history, &schema.Message{
		Role:    "user",
		Content: userInput + "\n", // 保留换行符
	})
	history = append(history, &schema.Message{
		Role:    "assistant",
		Content: assistantMsg + "\n",
	})

	return history
}

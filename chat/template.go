package main

import (
	"context"
	"log"

	"github.com/cloudwego/eino/components/prompt"
	"github.com/cloudwego/eino/schema"
)

var chatTemplate = prompt.FromMessages(schema.FString,
	schema.SystemMessage("你是一个{role}，{require}"),
	schema.MessagesPlaceholder("history", true),
	schema.UserMessage("{question}"),
)

// 生成消息，支持多轮历史
func buildMessages(history []*schema.Message, question string) []*schema.Message {
	variables := map[string]any{
		"role": "中英互译专家",
		"require": "你会对用户的任意中文输入翻译为英文，对用户的任意英文输入翻译为中文，" +
			"你需要考虑原文上下文的内容、原文的语言结构" +
			"原文的语气语境来确定目标语言的用词，确保译文的内容、语气和结构与原文保持一致，" +
			"并确保译文在目标语言中具有清晰、流畅的表达。你只需要返回翻译结果，请勿返回其他内容。",
		"question": question,
		"history":  history,
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
		Content: userInput,
	})
	history = append(history, &schema.Message{
		Role:    "assistant",
		Content: assistantMsg,
	})

	return history
}

package main

import (
	"context"
	"fmt"

	"github.com/cloudwego/eino/schema"
)

func main() {
	ctx := context.Background()
	cm := createOpenAIChatModel(ctx)

	var chatHistory []*schema.Message

	for {
		fmt.Print("\nuser: ")
		userInput := ReadUserInput()
		messages := buildMessages(chatHistory, userInput)

		// 流式获取回复
		streamResult := stream(ctx, cm, messages)
		assistantMsg := reportStream(streamResult)

		// 追加到历史
		chatHistory = AppendHistory(chatHistory, userInput, assistantMsg)
	}

}

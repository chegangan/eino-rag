package main

import (
	"context"
	"fmt"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
)

// Stream function to handle streaming responses from the chat model
func stream(ctx context.Context, llm model.ToolCallingChatModel, in []*schema.Message) (*schema.StreamReader[*schema.Message], error) {
	result, err := llm.Stream(ctx, in)
	if err != nil {
		return nil, fmt.Errorf("llm generate failed: %v", err)
	}
	return result, nil
}

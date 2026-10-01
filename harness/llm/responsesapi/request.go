package responsesapi

import (
	"bytes"
	"encoding/json/jsontext"
	"encoding/json/v2"
	"errors"
	"fmt"
	"slices"
	"sync"

	"github.com/viktordanov/unreal-agent/harness/llm"
	"github.com/viktordanov/unreal-agent/internal/openaiapi"
)

func requestBody(request llm.Request, promptCacheKey string, extensions map[string]jsontext.Value) ([]byte, error) {
	input, err := requestInput(request.Input)
	if err != nil {
		return nil, err
	}
	return encodeRequestBody(request, input, promptCacheKey, extensions)
}

func encodeRequestBody(
	request llm.Request,
	input requestInputItems,
	promptCacheKey string,
	extensions map[string]jsontext.Value,
) ([]byte, error) {
	tools, err := requestTools(request.Tools)
	if err != nil {
		return nil, err
	}

	var model openaiapi.ModelIdsResponses
	if err := model.FromModelIdsResponses1(openaiapi.ModelIdsResponses1(request.Model.ID)); err != nil {
		return nil, fmt.Errorf("encode model: %w", err)
	}
	store := false
	include := []openaiapi.IncludeEnum{openaiapi.ReasoningEncryptedContent}
	params := openaiapi.CreateResponse{
		Model:   &model,
		Store:   &store,
		Stream:  new(true),
		Include: &include,
	}
	if promptCacheKey != "" {
		params.PromptCacheKey = &promptCacheKey
	}
	if request.Model.MaxOutputTokens != nil {
		maxOutputTokens := int(*request.Model.MaxOutputTokens)
		params.MaxOutputTokens = &maxOutputTokens
	}
	if request.Model.ReasoningEffort != "" {
		if !request.Model.ReasoningEffort.Valid() {
			return nil, fmt.Errorf("unsupported reasoning effort %q", request.Model.ReasoningEffort)
		}
		effort := openaiapi.ReasoningEffort(request.Model.ReasoningEffort)
		summary := openaiapi.ReasoningSummaryAuto
		params.Reasoning = &openaiapi.Reasoning{
			Effort:  &effort,
			Summary: &summary,
		}
	}
	if len(tools) != 0 {
		params.Tools = &tools
	}
	if len(extensions) != 0 {
		return extendRequestBody(params, input, extensions)
	}
	if len(input) != 0 {
		// The placeholder is encoded from the items directly, so the history
		// is not copied into an intermediate union value first.
		params.Input = &openaiapi.InputParam{}
	}
	body := input.buffer()
	if err := json.MarshalWrite(body, params, json.Deterministic(true), json.WithMarshalers(
		json.MarshalToFunc(func(encoder *jsontext.Encoder, _ openaiapi.InputParam) error {
			return input.MarshalJSONTo(encoder)
		}),
	)); err != nil {
		return nil, fmt.Errorf("encode response request: %w", err)
	}
	return body.Bytes(), nil
}

func extendRequestBody(params openaiapi.CreateResponse, input requestInputItems, extensions map[string]jsontext.Value) ([]byte, error) {
	body, err := json.Marshal(params, json.Deterministic(true))
	if err != nil {
		return nil, fmt.Errorf("encode response request: %w", err)
	}
	var standard map[string]jsontext.Value
	if err := json.Unmarshal(body, &standard); err != nil {
		return nil, fmt.Errorf("decode response request: %w", err)
	}
	fields := make(map[string]any, len(standard)+1+len(extensions))
	for name, value := range standard {
		fields[name] = value
	}
	if len(input) != 0 {
		fields["input"] = input
	}
	for name, value := range extensions {
		if _, standard := fields[name]; standard {
			return nil, fmt.Errorf("request extension %q overrides a Responses API field", name)
		}
		if !value.IsValid() {
			return nil, fmt.Errorf("request extension %q is not valid JSON", name)
		}
		fields[name] = value
	}
	extended := input.buffer()
	if err := json.MarshalWrite(extended, fields, json.Deterministic(true)); err != nil {
		return nil, fmt.Errorf("encode response request: %w", err)
	}
	return extended.Bytes(), nil
}

// requestInputItems holds each input item encoded once, so building a request
// costs one pass over the history rather than one per nested union.
type requestInputItems []jsontext.Value

func (items requestInputItems) MarshalJSONTo(encoder *jsontext.Encoder) error {
	if err := encoder.WriteToken(jsontext.BeginArray); err != nil {
		return err
	}
	for _, item := range items {
		if err := encoder.WriteValue(item); err != nil {
			return err
		}
	}
	return encoder.WriteToken(jsontext.EndArray)
}

// buffer returns an empty buffer that holds the encoded items and the
// request's other fields without growing, since a large history would
// otherwise be copied each time the buffer grows.
func (items requestInputItems) buffer() *bytes.Buffer {
	size := 4 << 10
	for _, item := range items {
		size += len(item) + len(",")
	}
	// An encoder writing to a bytes.Buffer grows it whenever less than a
	// quarter of the written length remains available.
	return bytes.NewBuffer(make([]byte, 0, size+size/4))
}

func requestInput(items []llm.Item) (requestInputItems, error) {
	converted := make(requestInputItems, 0, len(items))
	for index, item := range items {
		input, err := requestInputItem(item)
		if err != nil {
			return nil, fmt.Errorf("input item %d: %w", index, err)
		}
		converted = append(converted, input)
	}
	return converted, nil
}

// inputCache keeps the previous request's input items and their encodings.
// Each request resends the conversation with new items appended, so only items
// that differ from the previous request's item at the same index are encoded.
// An item's encoding depends only on the item, and item data is not changed in
// place once built.
type inputCache struct {
	mu      sync.Mutex
	items   []llm.Item
	encoded requestInputItems
}

func (cache *inputCache) encode(items []llm.Item) (requestInputItems, error) {
	cache.mu.Lock()
	previous, previousEncoded := cache.items, cache.encoded
	cache.mu.Unlock()

	encoded := make(requestInputItems, 0, len(items))
	for index, item := range items {
		if index < len(previous) && sameItem(item, previous[index]) {
			encoded = append(encoded, previousEncoded[index])
			continue
		}
		input, err := requestInputItem(item)
		if err != nil {
			return nil, fmt.Errorf("input item %d: %w", index, err)
		}
		encoded = append(encoded, input)
	}

	cache.mu.Lock()
	cache.items, cache.encoded = slices.Clone(items), encoded
	cache.mu.Unlock()
	return encoded, nil
}

func sameItem(item, other llm.Item) bool {
	if item.ProviderID != other.ProviderID || item.Type != other.Type {
		return false
	}
	switch data := item.Data.(type) {
	case llm.Message:
		otherData, ok := other.Data.(llm.Message)
		return ok && data == otherData
	case llm.ToolCall:
		otherData, ok := other.Data.(llm.ToolCall)
		return ok && data == otherData
	case llm.ToolResult:
		otherData, ok := other.Data.(llm.ToolResult)
		return ok && data.CallID == otherData.CallID && slices.Equal(data.Output, otherData.Output)
	case llm.Reasoning:
		otherData, ok := other.Data.(llm.Reasoning)
		return ok && slices.Equal(data.Summary, otherData.Summary) && bytes.Equal(data.Raw, otherData.Raw)
	default:
		return false
	}
}

func requestInputItem(source llm.Item) (jsontext.Value, error) {
	if source.Type == llm.ItemMessage && source.ProviderID == "" {
		message, ok := source.Data.(llm.Message)
		if !ok {
			return nil, fmt.Errorf("message item data must be llm.Message, got %T", source.Data)
		}
		var content openaiapi.EasyInputMessage_Content
		if err := content.FromEasyInputMessageContent0(message.Text); err != nil {
			return nil, err
		}
		converted := openaiapi.EasyInputMessage{
			Content: content,
			Role:    openaiapi.EasyInputMessageRole(message.Role),
		}
		if message.Phase != "" {
			phase := openaiapi.MessagePhase(message.Phase)
			converted.Phase = &phase
		}
		return json.Marshal(converted, json.Deterministic(true))
	}

	converted, err := requestItem(source)
	if err != nil {
		return nil, err
	}
	return converted.MarshalJSON()
}

func requestItem(source llm.Item) (openaiapi.Item, error) {
	var item openaiapi.Item
	switch source.Type {
	case llm.ItemMessage:
		message, ok := source.Data.(llm.Message)
		if !ok {
			return item, fmt.Errorf("message item data must be llm.Message, got %T", source.Data)
		}
		if message.Role != llm.RoleAssistant {
			var content openaiapi.InputContent
			if err := setUnion(&content, openaiapi.InputTextContent{
				Text: message.Text,
				Type: openaiapi.InputTextContentTypeInputText,
			}); err != nil {
				return item, err
			}
			messageType := openaiapi.InputMessageTypeMessage
			converted := openaiapi.InputMessage{
				Content: []openaiapi.InputContent{content},
				Role:    openaiapi.InputMessageRole(message.Role),
				Type:    &messageType,
			}
			if err := setUnion(&item, converted); err != nil {
				return item, err
			}
			return item, nil
		}
		content, err := requestOutputMessageContent(message)
		if err != nil {
			return item, err
		}
		converted := openaiapi.OutputMessage{
			Content: content,
			Id:      source.ProviderID,
			Role:    openaiapi.OutputMessageRoleAssistant,
			Status:  openaiapi.OutputMessageStatusCompleted,
			Type:    openaiapi.OutputMessageTypeMessage,
		}
		if message.Phase != "" {
			phase := openaiapi.MessagePhase(message.Phase)
			converted.Phase = &phase
		}
		if err := setUnion(&item, converted); err != nil {
			return item, err
		}
	case llm.ItemToolCall:
		call, ok := source.Data.(llm.ToolCall)
		if !ok {
			return item, fmt.Errorf("tool_call item data must be llm.ToolCall, got %T", source.Data)
		}
		arguments, err := requestToolCallArguments(call.Arguments)
		if err != nil {
			return item, fmt.Errorf("encode tool call %q arguments: %w", call.CallID, err)
		}
		converted := openaiapi.FunctionToolCall{
			Arguments: arguments,
			CallId:    call.CallID,
			Name:      call.Name,
			Type:      openaiapi.FunctionCall,
		}
		if source.ProviderID != "" {
			converted.Id = &source.ProviderID
		}
		if err := setUnion(&item, converted); err != nil {
			return item, err
		}
	case llm.ItemToolResult:
		output, ok := source.Data.(llm.ToolResult)
		if !ok {
			return item, fmt.Errorf("tool_result item data must be llm.ToolResult, got %T", source.Data)
		}
		contents := make(openaiapi.FunctionCallOutputItemParamOutput1, len(output.Output))
		for i, part := range output.Output {
			var content any
			switch part.Kind {
			case llm.ToolResultText:
				content = openaiapi.InputTextContentParam{
					Text: part.Value,
					Type: openaiapi.InputTextContentParamTypeInputText,
				}
			case llm.ToolResultImage:
				content = openaiapi.InputImageContentParamAutoParam{
					ImageUrl: &part.Value,
					Type:     openaiapi.InputImageContentParamAutoParamTypeInputImage,
				}
			default:
				return item, fmt.Errorf("unsupported tool result kind %q", part.Kind)
			}
			if err := setUnion(&contents[i], content); err != nil {
				return item, err
			}
		}
		var value openaiapi.FunctionCallOutputItemParam_Output
		if err := setUnion(&value, contents); err != nil {
			return item, err
		}
		converted := openaiapi.FunctionCallOutputItemParam{
			CallId: &output.CallID,
			Output: value,
			Type:   openaiapi.FunctionCallOutputItemParamTypeFunctionCallOutput,
		}
		if err := setUnion(&item, converted); err != nil {
			return item, err
		}
	case llm.ItemReasoning:
		reasoning, ok := source.Data.(llm.Reasoning)
		if !ok {
			return item, fmt.Errorf("reasoning item data must be llm.Reasoning, got %T", source.Data)
		}
		if len(reasoning.Raw) == 0 {
			return item, errors.New("reasoning item must carry the provider item in Raw")
		}
		if err := setUnion(&item, reasoning.Raw); err != nil {
			return item, err
		}
	default:
		return item, fmt.Errorf("unsupported input item type %q", source.Type)
	}
	return item, nil
}

func requestToolCallArguments(arguments string) (string, error) {
	value := jsontext.Value(arguments)
	if value.Kind() == jsontext.KindBeginObject && value.IsValid() {
		return arguments, nil
	}
	// Providers may reject their own malformed calls in history. Keep the raw
	// arguments visible alongside the tool error in a replayable JSON object.
	encoded, err := json.Marshal(struct {
		InvalidArguments string `json:"invalid_arguments"`
	}{InvalidArguments: arguments})
	return string(encoded), err
}

func requestOutputMessageContent(message llm.Message) ([]openaiapi.OutputMessageContent, error) {
	content := make([]openaiapi.OutputMessageContent, 0, 1)
	if message.Text != "" {
		var part openaiapi.OutputMessageContent
		if err := setUnion(&part, openaiapi.OutputTextContent{
			Annotations: []openaiapi.Annotation{},
			Logprobs:    []openaiapi.LogProb{},
			Text:        message.Text,
			Type:        openaiapi.OutputText,
		}); err != nil {
			return nil, err
		}
		content = append(content, part)
	}
	return content, nil
}

func requestTools(source []llm.Tool) (openaiapi.ToolsArray, error) {
	tools := make(openaiapi.ToolsArray, 0, len(source))
	for index, source := range source {
		tool, err := requestTool(source)
		if err != nil {
			return nil, fmt.Errorf("tool %d: %w", index, err)
		}
		tools = append(tools, tool)
	}
	return tools, nil
}

func requestTool(source llm.Tool) (openaiapi.Tool, error) {
	var converted any
	switch source.Type {
	case llm.ToolFunction:
		parameters := source.Parameters
		// The wire requires the field. The harness validates tool calls itself, and provider
		// schema enforcement would reject the loose schemas that external tools contribute.
		strict := false
		function := openaiapi.FunctionTool{
			Name:       source.Name,
			Parameters: &parameters,
			Strict:     &strict,
			Type:       openaiapi.FunctionToolTypeFunction,
		}
		if source.Description != "" {
			description := source.Description
			function.Description = &description
		}
		converted = function
	case llm.ToolHosted:
		switch source.Name {
		case "web_search":
			converted = openaiapi.WebSearchTool{Type: openaiapi.WebSearch}
		default:
			return openaiapi.Tool{}, fmt.Errorf("unsupported hosted tool name %q", source.Name)
		}
	default:
		return openaiapi.Tool{}, fmt.Errorf("unsupported tool type %q", source.Type)
	}

	var tool openaiapi.Tool
	if err := setUnion(&tool, converted); err != nil {
		return openaiapi.Tool{}, err
	}
	return tool, nil
}

func setUnion(destination json.Unmarshaler, source any) error {
	body, err := json.Marshal(source, json.Deterministic(true))
	if err != nil {
		return err
	}
	return destination.UnmarshalJSON(body)
}

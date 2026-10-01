package responsesapi

import (
	"bytes"
	"encoding/json/jsontext"
	"encoding/json/v2"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/unreallabsai/unreal-agent/harness/llm"
	"github.com/unreallabsai/unreal-agent/internal/openaiapi"
)

func TestRequestBodyMatchesLegacyEncoding(t *testing.T) {
	response, err := decodeResponse([]byte(`{
		"id":"response-1","status":"completed","output":[
			{"type":"reasoning","id":"rs_1","summary":[{"type":"summary_text","text":"Plan \u003cfirst\u003e"}],
				"encrypted_content":"gAAAAA\/x+=","status" : "completed"},
			{"type":"function_call","id":"fc_1","call_id":"call-1","name":"Bash","arguments":"{\"command\": \"ls <dir> && echo \\u00e9\"}"},
			{"type":"function_call","id":"fc_2","call_id":"call-2","name":"Bash","arguments":"{\"command\":\"apt-get"},
			{"type":"message","id":"msg_1","role":"assistant","phase":"commentary","status":"completed",
				"content":[{"type":"output_text","text":"Listing \u2028 files & <tags>","annotations":[]}]}
		]
	}`))
	if err != nil {
		t.Fatal(err)
	}
	history := []llm.Item{
		{Type: llm.ItemMessage, Data: llm.Message{Role: llm.RoleSystem, Text: "System <rules> & \"quotes\"\n\t\u2029"}},
		{Type: llm.ItemMessage, Data: llm.Message{Role: llm.RoleUser, Text: "héllo 👋 \\ \u0000 </script>"}},
		{Type: llm.ItemMessage, Data: llm.Message{Role: llm.RoleUser, Text: "steer", Phase: "commentary"}},
		{ProviderID: "msg_0", Type: llm.ItemMessage, Data: llm.Message{Role: llm.RoleUser, Text: "identified <user>"}},
		{ProviderID: "msg_s", Type: llm.ItemMessage, Data: llm.Message{Role: llm.RoleSystem, Text: "identified system"}},
		{ProviderID: "msg_a", Type: llm.ItemMessage, Data: llm.Message{Role: llm.RoleAssistant}},
		{Type: llm.ItemReasoning, Data: llm.Reasoning{Raw: jsontext.Value(" {\"type\" : \"reasoning\",\n\"id\":\"rs_0\", \"x\":\"\\u003c\\/\\u00e9\", \"n\": 1.50e+2} ")}},
	}
	history = append(history, response.Output...)
	history = append(history,
		llm.Item{Type: llm.ItemToolResult, Data: llm.ToolResult{CallID: "call-1", Output: []llm.ToolResultOutput{
			{Kind: llm.ToolResultText, Value: "total 0\n<dir> & more"},
			{Kind: llm.ToolResultImage, Value: "data:image/png;base64,iVBORw0KGgo="},
		}}},
		llm.Item{Type: llm.ItemToolResult, Data: llm.ToolResult{CallID: "call-2"}},
		llm.Item{Type: llm.ItemToolCall, Data: llm.ToolCall{CallID: "call-3", Name: "Bash", Arguments: " \n{\"a\": [1, 2]} "}},
	)
	maxOutputTokens := int64(4096)
	tools := []llm.Tool{
		{Type: llm.ToolFunction, Name: "Bash", Description: "Run <a> command", Parameters: map[string]any{
			"type": "object", "properties": map[string]any{"command": map[string]any{"type": "string"}}}},
		{Type: llm.ToolHosted, Name: "web_search"},
	}
	extensions := map[string]jsontext.Value{
		"provider": jsontext.Value(`{"sort" : "throughput", "only":["a<b"]}`),
		"zz":       jsontext.Value(`true`),
	}
	tests := []struct {
		name           string
		request        llm.Request
		promptCacheKey string
		extensions     map[string]jsontext.Value
	}{
		{name: "empty"},
		{name: "empty with extensions", extensions: extensions},
		{name: "history", request: llm.Request{Input: history}},
		{
			name: "history with options",
			request: llm.Request{
				Model: llm.Model{ID: "gpt-test", ReasoningEffort: llm.ReasoningEffortHigh, MaxOutputTokens: &maxOutputTokens},
				Input: history,
				Tools: tools,
			},
			promptCacheKey: "cache-key",
		},
		{
			name:           "history with extensions",
			request:        llm.Request{Model: llm.Model{ID: "gpt-test"}, Input: history, Tools: tools},
			promptCacheKey: "cache-key",
			extensions:     extensions,
		},
		{name: "benchmark history", request: benchmarkRequest(20)},
		{name: "input extension", request: llm.Request{Input: history}, extensions: map[string]jsontext.Value{"input": jsontext.Value(`[]`)}},
		{name: "input extension without input", extensions: map[string]jsontext.Value{"input": jsontext.Value(`[]`)}},
		{name: "invalid extension", request: llm.Request{Input: history}, extensions: map[string]jsontext.Value{"x": jsontext.Value(`{`)}},
		{name: "unsupported item", request: llm.Request{Input: append(history[:2:2], llm.Item{Type: "image"})}},
		{name: "invalid reasoning", request: llm.Request{Input: []llm.Item{{Type: llm.ItemReasoning, Data: llm.Reasoning{Raw: jsontext.Value(`{"type":`)}}}}},
		{name: "invalid UTF-8", request: llm.Request{Input: []llm.Item{{Type: llm.ItemMessage, Data: llm.Message{Role: llm.RoleUser, Text: "\xff"}}}}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			want, wantErr := legacyRequestBody(test.request, test.promptCacheKey, test.extensions)
			got, err := requestBody(test.request, test.promptCacheKey, test.extensions)
			if (err == nil) != (wantErr == nil) || err != nil && err.Error() != wantErr.Error() {
				t.Fatalf("error = %v, want %v", err, wantErr)
			}
			if !bytes.Equal(got, want) {
				t.Fatalf("body =\n%s\nwant\n%s", got, want)
			}
		})
	}
}

func TestInputCacheMatchesLegacyEncoding(t *testing.T) {
	history := benchmarkRequest(6).Input
	edited := slices.Clone(history)
	edited[3] = llm.Item{Type: llm.ItemMessage, Data: llm.Message{Role: llm.RoleUser, Text: "edited"}}
	result := history[4].Data.(llm.ToolResult)
	result.Output = []llm.ToolResultOutput{{Kind: llm.ToolResultText, Value: result.Output[0].Value + "more"}}
	changedResult := slices.Clone(history)
	changedResult[4] = llm.Item{Type: llm.ItemToolResult, Data: result}
	reasoning := history[2].Data.(llm.Reasoning)
	reasoning.Raw = slices.Clone(reasoning.Raw)
	reasoning.Raw[len(reasoning.Raw)-3] = 'x'
	changedReasoning := slices.Clone(history)
	changedReasoning[2] = llm.Item{Type: llm.ItemReasoning, Data: reasoning}
	providerID := slices.Clone(history)
	providerID[3].ProviderID = "msg_new"
	steps := []struct {
		name  string
		input []llm.Item
		fails bool
	}{
		{name: "first", input: history[:6]},
		{name: "appended", input: history[:16]},
		{name: "unchanged", input: history[:16]},
		{name: "rewound", input: history[:11]},
		{name: "edited", input: edited},
		{name: "unsupported", input: append(slices.Clone(history), llm.Item{Type: "image"}), fails: true},
		{name: "after failure", input: history},
		{name: "changed tool result", input: changedResult},
		{name: "changed reasoning", input: changedReasoning},
		{name: "changed provider ID", input: providerID},
		{name: "empty"},
		{name: "full", input: history},
	}
	var cache inputCache
	for _, step := range steps {
		request := llm.Request{Model: llm.Model{ID: "gpt-test"}, Input: step.input}
		want, wantErr := legacyRequestBody(request, "", nil)
		input, err := cache.encode(step.input)
		if (err != nil) != step.fails || (wantErr != nil) != step.fails {
			t.Fatalf("%s: error = %v, legacy error = %v", step.name, err, wantErr)
		}
		if step.fails {
			if err.Error() != wantErr.Error() {
				t.Fatalf("%s: error = %v, want %v", step.name, err, wantErr)
			}
			continue
		}
		got, err := encodeRequestBody(request, input, "", nil)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(got, want) {
			t.Fatalf("%s: body =\n%s\nwant\n%s", step.name, got, want)
		}
	}
}

func TestInputCacheReusesUnchangedItems(t *testing.T) {
	history := benchmarkRequest(2).Input
	var cache inputCache
	first, err := cache.encode(history[:6])
	if err != nil {
		t.Fatal(err)
	}
	second, err := cache.encode(history)
	if err != nil {
		t.Fatal(err)
	}
	for index := range first {
		if &first[index][0] != &second[index][0] {
			t.Fatalf("item %d was encoded again", index)
		}
	}
}

func BenchmarkRequestBody(b *testing.B) {
	extensions := map[string]jsontext.Value{"provider": jsontext.Value(`{"sort":"throughput"}`)}
	for _, turns := range []int{100, 1000} {
		request := benchmarkRequest(turns)
		for _, extended := range []bool{false, true} {
			current := map[string]jsontext.Value(nil)
			if extended {
				current = extensions
			}
			body, err := requestBody(request, "cache-key", current)
			if err != nil {
				b.Fatal(err)
			}
			b.Run(fmt.Sprintf("turns=%d/extensions=%t", turns, extended), func(b *testing.B) {
				b.SetBytes(int64(len(body)))
				b.ReportAllocs()
				for b.Loop() {
					if _, err := requestBody(request, "cache-key", current); err != nil {
						b.Fatal(err)
					}
				}
			})
		}
		// An adapter's next request: the previous request's history plus one turn.
		previous := request.Input[:len(request.Input)-5]
		var warm inputCache
		if _, err := warm.encode(previous); err != nil {
			b.Fatal(err)
		}
		body, err := requestBody(request, "cache-key", nil)
		if err != nil {
			b.Fatal(err)
		}
		b.Run(fmt.Sprintf("turns=%d/next", turns), func(b *testing.B) {
			b.SetBytes(int64(len(body)))
			b.ReportAllocs()
			for b.Loop() {
				cache := inputCache{items: warm.items, encoded: warm.encoded}
				input, err := cache.encode(request.Input)
				if err != nil {
					b.Fatal(err)
				}
				if _, err := encodeRequestBody(request, input, "cache-key", nil); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

// benchmarkRequest returns a coding-agent history: each turn is a reasoning item
// with encrypted content, a tool call, a large tool result, and a short reply.
func benchmarkRequest(turns int) llm.Request {
	output := strings.Repeat("drwxr-xr-x  5 user  staff   160 Jan  1 00:00 <dir> & \"file\"\n", 160)
	encrypted := strings.Repeat("gAAAAABo", 512)
	request := llm.Request{
		Model: llm.Model{ID: "gpt-test", ReasoningEffort: llm.ReasoningEffortHigh},
		Input: []llm.Item{{
			Type: llm.ItemMessage,
			Data: llm.Message{Role: llm.RoleSystem, Text: strings.Repeat("You are a coding agent. ", 400)},
		}},
		Tools: []llm.Tool{{Type: llm.ToolFunction, Name: "Bash", Description: "Run a command.", Parameters: map[string]any{"type": "object"}}},
	}
	for turn := range turns {
		request.Input = append(request.Input,
			llm.Item{Type: llm.ItemMessage, Data: llm.Message{Role: llm.RoleUser, Text: fmt.Sprintf("Step %d: list the files.", turn)}},
			llm.Item{Type: llm.ItemReasoning, Data: llm.Reasoning{Raw: jsontext.Value(fmt.Sprintf(
				`{"id":"rs_%d","type":"reasoning","summary":[],"encrypted_content":%q}`, turn, encrypted))}},
			llm.Item{ProviderID: fmt.Sprintf("fc_%d", turn), Type: llm.ItemToolCall, Data: llm.ToolCall{
				CallID: fmt.Sprintf("call_%d", turn), Name: "Bash", Arguments: `{"command":"ls -la"}`}},
			llm.Item{Type: llm.ItemToolResult, Data: llm.ToolResult{
				CallID: fmt.Sprintf("call_%d", turn), Output: []llm.ToolResultOutput{{Kind: llm.ToolResultText, Value: output}}}},
			llm.Item{ProviderID: fmt.Sprintf("msg_%d", turn), Type: llm.ItemMessage, Data: llm.Message{
				Role: llm.RoleAssistant, Text: "Listed the files.", Phase: "final_answer"}},
		)
	}
	return request
}

// legacyRequestBody is requestBody before input items were encoded once. It
// encoded every item into nested unions and re-encoded the whole history at
// each level, and remains here to show that the request bytes are unchanged.
func legacyRequestBody(request llm.Request, promptCacheKey string, extensions map[string]jsontext.Value) ([]byte, error) {
	input, err := legacyRequestInput(request.Input)
	if err != nil {
		return nil, err
	}
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
		Input:   &input,
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
	body, err := json.Marshal(params, json.Deterministic(true))
	if err != nil {
		return nil, fmt.Errorf("encode response request: %w", err)
	}
	if len(extensions) == 0 {
		return body, nil
	}
	return legacyExtendRequestBody(body, extensions)
}

func legacyExtendRequestBody(body []byte, extensions map[string]jsontext.Value) ([]byte, error) {
	var fields map[string]jsontext.Value
	if err := json.Unmarshal(body, &fields); err != nil {
		return nil, fmt.Errorf("decode response request: %w", err)
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
	extended, err := json.Marshal(fields, json.Deterministic(true))
	if err != nil {
		return nil, fmt.Errorf("encode response request: %w", err)
	}
	return extended, nil
}

func legacyRequestInput(items []llm.Item) (openaiapi.InputParam, error) {
	converted := make(openaiapi.InputParam1, 0, len(items))
	for index, item := range items {
		input, err := legacyRequestInputItem(item)
		if err != nil {
			return openaiapi.InputParam{}, fmt.Errorf("input item %d: %w", index, err)
		}
		converted = append(converted, input)
	}

	var input openaiapi.InputParam
	if err := input.FromInputParam1(converted); err != nil {
		return openaiapi.InputParam{}, fmt.Errorf("encode input: %w", err)
	}
	return input, nil
}

func legacyRequestInputItem(source llm.Item) (openaiapi.InputItem, error) {
	var item openaiapi.InputItem
	if source.Type == llm.ItemMessage && source.ProviderID == "" {
		message, ok := source.Data.(llm.Message)
		if !ok {
			return item, fmt.Errorf("message item data must be llm.Message, got %T", source.Data)
		}
		var content openaiapi.EasyInputMessage_Content
		if err := content.FromEasyInputMessageContent0(message.Text); err != nil {
			return item, err
		}
		converted := openaiapi.EasyInputMessage{
			Content: content,
			Role:    openaiapi.EasyInputMessageRole(message.Role),
		}
		if message.Phase != "" {
			phase := openaiapi.MessagePhase(message.Phase)
			converted.Phase = &phase
		}
		if err := setUnion(&item, converted); err != nil {
			return item, err
		}
		return item, nil
	}

	converted, err := requestItem(source)
	if err != nil {
		return item, err
	}
	if err := setUnion(&item, converted); err != nil {
		return item, err
	}
	return item, nil
}

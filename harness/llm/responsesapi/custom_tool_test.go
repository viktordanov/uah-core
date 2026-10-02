package responsesapi

import (
	"encoding/json/jsontext"
	"encoding/json/v2"
	"reflect"
	"testing"

	"github.com/viktordanov/uah-core/harness/llm"
)

const testPatch = "*** Begin Patch\n*** Add File: a.txt\n+say \"hi\"\n*** End Patch"

func TestRequestBodyEncodesCustomTools(t *testing.T) {
	body, err := requestBody(llm.Request{
		Model: llm.Model{ID: "gpt-test"},
		Tools: []llm.Tool{
			{Type: llm.ToolCustom, Name: "apply_patch", Description: "Edit files.", Grammar: &llm.ToolGrammar{
				Syntax: "lark", Definition: "start: \"x\"\n",
			}},
			{Type: llm.ToolCustom, Name: "notes"},
		},
	}, "", nil)
	if err != nil {
		t.Fatal(err)
	}
	var request struct {
		Tools []jsontext.Value `json:"tools"`
	}
	if err := json.Unmarshal(body, &request); err != nil {
		t.Fatal(err)
	}
	// The shapes Codex sends for a freeform tool.
	want := []string{
		`{"type":"custom","name":"apply_patch","description":"Edit files.",` +
			`"format":{"type":"grammar","syntax":"lark","definition":"start: \"x\"\n"}}`,
		`{"type":"custom","name":"notes"}`,
	}
	if len(request.Tools) != len(want) {
		t.Fatalf("tools = %s", request.Tools)
	}
	for index, tool := range request.Tools {
		assertSameJSON(t, tool, want[index])
	}
}

func TestRequestBodyPairsCustomToolCallsWithTheirOutputs(t *testing.T) {
	result := func(callID string) llm.Item {
		return llm.Item{Type: llm.ItemToolResult, Data: llm.ToolResult{CallID: callID, Output: []llm.ToolResultOutput{
			{Kind: llm.ToolResultText, Value: "Done!"},
		}}}
	}
	body, err := requestBody(llm.Request{Input: []llm.Item{
		{ProviderID: "ctc_1", Type: llm.ItemToolCall, Data: llm.ToolCall{
			CallID: "call-1", Name: "apply_patch", Arguments: testPatch, Custom: true,
		}},
		{Type: llm.ItemToolCall, Data: llm.ToolCall{CallID: "call-2", Name: "Bash", Arguments: `{}`}},
		{Type: llm.ItemToolCall, Data: llm.ToolCall{CallID: "call-3", Name: "apply_patch", Arguments: "", Custom: true}},
		result("call-1"),
		result("call-2"),
		result("call-3"),
	}}, "", nil)
	if err != nil {
		t.Fatal(err)
	}
	var request struct {
		Input []jsontext.Value `json:"input"`
	}
	if err := json.Unmarshal(body, &request); err != nil {
		t.Fatal(err)
	}
	output := `[{"type":"input_text","text":"Done!"}]`
	want := []string{
		`{"type":"custom_tool_call","id":"ctc_1","call_id":"call-1","name":"apply_patch","input":` + quote(t, testPatch) + `}`,
		`{"type":"function_call","call_id":"call-2","name":"Bash","arguments":"{}"}`,
		`{"type":"custom_tool_call","call_id":"call-3","name":"apply_patch","input":""}`,
		`{"type":"custom_tool_call_output","call_id":"call-1","output":` + output + `}`,
		`{"type":"function_call_output","call_id":"call-2","output":` + output + `}`,
		`{"type":"custom_tool_call_output","call_id":"call-3","output":` + output + `}`,
	}
	if len(request.Input) != len(want) {
		t.Fatalf("input = %s", request.Input)
	}
	for index, item := range request.Input {
		assertSameJSON(t, item, want[index])
	}
}

func TestInputCacheReencodesResultWhenItsCallKindChanges(t *testing.T) {
	result := llm.Item{Type: llm.ItemToolResult, Data: llm.ToolResult{CallID: "call-1"}}
	function := []llm.Item{
		{Type: llm.ItemToolCall, Data: llm.ToolCall{CallID: "call-1", Name: "apply_patch", Arguments: `{}`}},
		result,
	}
	custom := []llm.Item{
		{Type: llm.ItemToolCall, Data: llm.ToolCall{CallID: "call-1", Name: "apply_patch", Arguments: `{}`, Custom: true}},
		result,
	}
	var cache inputCache
	for _, input := range [][]llm.Item{function, custom, custom, function} {
		got, err := cache.encode(input)
		if err != nil {
			t.Fatal(err)
		}
		want, err := requestInput(input)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("cached input =\n%s\nwant\n%s", got, want)
		}
	}
}

func TestResponseReadsCustomToolCalls(t *testing.T) {
	response, err := decodeResponse([]byte(`{
		"id":"response-1","status":"completed","output":[
			{"type":"custom_tool_call","id":"ctc_1","status":"completed","call_id":"call-1","name":"apply_patch",
				"input":` + quote(t, testPatch) + `},
			{"type":"custom_tool_call","id":"ctc_2","call_id":"call-2","name":"apply_patch","input":""},
			{"type":"custom_tool_call","id":"ctc_3","status":"in_progress","call_id":"call-3","name":"apply_patch","input":"*** Begin"},
			{"type":"custom_tool_call","id":"ctc_4","status":"incomplete","call_id":"call-4","name":"apply_patch","input":"*** Begin"}
		]
	}`))
	if err != nil {
		t.Fatal(err)
	}
	want := []llm.Item{
		{ProviderID: "ctc_1", Type: llm.ItemToolCall, Data: llm.ToolCall{
			CallID: "call-1", Name: "apply_patch", Arguments: testPatch, Custom: true,
		}},
		{ProviderID: "ctc_2", Type: llm.ItemToolCall, Data: llm.ToolCall{
			CallID: "call-2", Name: "apply_patch", Custom: true,
		}},
	}
	if !reflect.DeepEqual(response.Output, want) {
		t.Fatalf("output = %#v, want %#v", response.Output, want)
	}
}

func TestCustomToolCallReplaysFromItsResponse(t *testing.T) {
	item := `{"type":"custom_tool_call","id":"ctc_1","call_id":"call-1","name":"apply_patch","input":` +
		quote(t, testPatch) + `}`
	response, err := decodeResponse([]byte(`{"id":"response-1","status":"completed","output":[` + item + `]}`))
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := requestInputItem(response.Output[0], customCalls(response.Output))
	if err != nil {
		t.Fatal(err)
	}
	assertSameJSON(t, encoded, item)
}

func quote(t *testing.T, text string) string {
	t.Helper()
	encoded, err := json.Marshal(text)
	if err != nil {
		t.Fatal(err)
	}
	return string(encoded)
}

func assertSameJSON(t *testing.T, got jsontext.Value, want string) {
	t.Helper()
	var gotValue, wantValue any
	if err := json.Unmarshal(got, &gotValue); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal([]byte(want), &wantValue); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(gotValue, wantValue) {
		t.Fatalf("JSON = %s\nwant %s", got, want)
	}
}

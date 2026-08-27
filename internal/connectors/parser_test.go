package connectors_test

import (
	"testing"

	"github.com/luoyif/memory-harness/internal/connectors"
)

func TestChatGPTActiveBranchParsing(t *testing.T) {
	payload := []byte(`[{"id":"c1","title":"Branch","create_time":1787260000,"update_time":1787260100,"current_node":"a2","mapping":{"root":{"id":"root","parent":"","message":null},"u1":{"id":"u1","parent":"root","message":{"id":"u1","author":{"role":"user"},"create_time":1787260001,"content":{"content_type":"text","parts":["用户问题"]}}},"a1":{"id":"a1","parent":"u1","message":{"id":"a1","author":{"role":"assistant"},"create_time":1787260002,"content":{"content_type":"text","parts":["旧回答"]}}},"a2":{"id":"a2","parent":"u1","message":{"id":"a2","author":{"role":"assistant"},"create_time":1787260003,"content":{"content_type":"text","parts":["当前回答"]}}}}}]`)
	items, preview, err := connectors.Parse("chatgpt", payload)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || len(items[0].Messages) != 2 || items[0].Messages[1].Text != "当前回答" || preview.Messages != 2 {
		t.Fatalf("items=%#v preview=%#v", items, preview)
	}
}

func TestClaudeThinkingIsPreservedOnlyInRawWarning(t *testing.T) {
	payload := []byte(`[{"uuid":"c1","name":"Claude export","created_at":"2026-08-20T00:00:00Z","updated_at":"2026-08-20T00:01:00Z","chat_messages":[{"uuid":"m1","sender":"human","created_at":"2026-08-20T00:00:00Z","text":"问题"},{"uuid":"m2","sender":"assistant","created_at":"2026-08-20T00:01:00Z","content":[{"type":"thinking","thinking":"private chain"},{"type":"text","text":"可见回答"}]}]}]`)
	items, preview, err := connectors.Parse("claude", payload)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || len(items[0].Messages) != 2 || items[0].Messages[1].Text != "可见回答" || len(items[0].Messages[1].Warnings) != 1 || preview.Warnings != 1 {
		t.Fatalf("items=%#v preview=%#v", items, preview)
	}
}

func TestDeepSeekVisibleFragmentsAndThinkingBoundary(t *testing.T) {
	payload := []byte(`[{"id":"d1","title":"DeepSeek export","inserted_at":"2026-08-20T00:00:00Z","updated_at":"2026-08-20T00:01:00Z","mapping":{"root":{"id":"root","children":["m1"],"message":null},"m1":{"id":"m1","children":["m2"],"message":{"inserted_at":"2026-08-20T00:00:00Z","fragments":[{"type":"REQUEST","content":"查资料"}]}},"m2":{"id":"m2","children":[],"message":{"inserted_at":"2026-08-20T00:01:00Z","fragments":[{"type":"THINK","content":"private"},{"type":"SEARCH","results":[{"title":"Memory","snippet":"source","url":"https://example.com"}]},{"type":"RESPONSE","content":"最终回答"}]}}}}]`)
	items, preview, err := connectors.Parse("deepseek", payload)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || len(items[0].Messages) != 2 || len(items[0].Messages[1].Warnings) != 1 || preview.Messages != 2 || items[0].Messages[1].Text == "private" {
		t.Fatalf("items=%#v preview=%#v", items, preview)
	}
}

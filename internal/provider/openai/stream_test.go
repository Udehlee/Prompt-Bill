package openai

import (
	"strings"
	"testing"
)

func TestStreamParser_StandardUsage(t *testing.T) {
	parser := NewStreamParser()

	rawSSE := `data: {"id":"chatcmpl-1","model":"gpt-4o-mini-2024-07-18","choices":[{"delta":{"content":"Hello"}}]}

data: {"id":"chatcmpl-1","model":"gpt-4o-mini-2024-07-18","choices":[{"delta":{"content":" world!"}}]}

data: {"id":"chatcmpl-1","model":"gpt-4o-mini-2024-07-18","choices":[],"usage":{"prompt_tokens":15,"completion_tokens":5,"total_tokens":20,"prompt_tokens_details":{"cached_tokens":3}}}

data: [DONE]

`
	parser.Feed([]byte(rawSSE))
	res := parser.Finalize()

	if !res.SawDone {
		t.Errorf("expected sawDone to be true")
	}
	if res.ReportedModel != "gpt-4o-mini-2024-07-18" {
		t.Errorf("expected reported model 'gpt-4o-mini-2024-07-18', got %s", res.ReportedModel)
	}
	if res.ExtractedUsage == nil {
		t.Fatalf("expected extracted usage, got nil")
	}
	if res.ExtractedUsage.UsageStatus != "complete" {
		t.Errorf("expected usage status 'complete', got %s", res.ExtractedUsage.UsageStatus)
	}
	if res.ExtractedUsage.InputTokens == nil || *res.ExtractedUsage.InputTokens != 15 {
		t.Errorf("expected 15 input tokens, got %v", res.ExtractedUsage.InputTokens)
	}
	if res.ExtractedUsage.OutputTokens == nil || *res.ExtractedUsage.OutputTokens != 5 {
		t.Errorf("expected 5 output tokens, got %v", res.ExtractedUsage.OutputTokens)
	}
	if res.ExtractedUsage.CachedTokens == nil || *res.ExtractedUsage.CachedTokens != 3 {
		t.Errorf("expected 3 cached tokens, got %v", res.ExtractedUsage.CachedTokens)
	}
	if res.ExtractedUsage.TotalTokens == nil || *res.ExtractedUsage.TotalTokens != 20 {
		t.Errorf("expected 20 total tokens, got %v", res.ExtractedUsage.TotalTokens)
	}
	if res.OversizedEvent || res.MalformedJSON {
		t.Errorf("unexpected error flags: oversized=%v, malformed=%v", res.OversizedEvent, res.MalformedJSON)
	}
}

func TestStreamParser_SplitAcrossNetworkReads(t *testing.T) {
	parser := NewStreamParser()

	rawSSE := `data: {"id":"chatcmpl-1","model":"gpt-4o-mini","choices":[{"delta":{"content":"chunk"}}]}

data: {"id":"chatcmpl-1","model":"gpt-4o-mini","choices":[],"usage":{"prompt_tokens":10,"completion_tokens":4,"total_tokens":14}}

data: [DONE]

`
	// Feed 1 byte at a time to test arbitrary network packet fragmentation
	for i := 0; i < len(rawSSE); i++ {
		parser.Feed([]byte{rawSSE[i]})
	}

	res := parser.Finalize()

	if !res.SawDone {
		t.Errorf("expected sawDone to be true")
	}
	if res.ReportedModel != "gpt-4o-mini" {
		t.Errorf("expected model 'gpt-4o-mini', got %s", res.ReportedModel)
	}
	if res.ExtractedUsage == nil {
		t.Fatalf("expected extracted usage, got nil")
	}
	if *res.ExtractedUsage.InputTokens != 10 || *res.ExtractedUsage.OutputTokens != 4 {
		t.Errorf("unexpected token counts: in=%v, out=%v", *res.ExtractedUsage.InputTokens, *res.ExtractedUsage.OutputTokens)
	}
}

func TestStreamParser_MissingFinalUsage(t *testing.T) {
	parser := NewStreamParser()

	// Client omitted stream_options: {"include_usage": true}, so OpenAI does not send usage
	rawSSE := `data: {"id":"chatcmpl-1","model":"gpt-4o","choices":[{"delta":{"content":"Hello"}}]}

data: [DONE]

`
	parser.Feed([]byte(rawSSE))
	res := parser.Finalize()

	if !res.SawDone {
		t.Errorf("expected sawDone to be true")
	}
	if res.ReportedModel != "gpt-4o" {
		t.Errorf("expected reported model 'gpt-4o', got %s", res.ReportedModel)
	}
	if res.ExtractedUsage != nil {
		t.Errorf("expected nil extracted usage when usage chunk is missing, got %+v", res.ExtractedUsage)
	}
}

func TestStreamParser_MalformedJSON(t *testing.T) {
	parser := NewStreamParser()

	rawSSE := `data: {invalid json

data: [DONE]

`
	parser.Feed([]byte(rawSSE))
	res := parser.Finalize()

	if !res.MalformedJSON {
		t.Errorf("expected MalformedJSON flag to be true")
	}
	if !res.SawDone {
		t.Errorf("expected sawDone to be true")
	}
	if res.ExtractedUsage != nil {
		t.Errorf("expected nil extracted usage, got %+v", res.ExtractedUsage)
	}
}

func TestStreamParser_EmptyLinesAndComments(t *testing.T) {
	parser := NewStreamParser()

	rawSSE := `: keep-alive comment
: another comment

data: {"id":"chatcmpl-1","model":"gpt-4o","choices":[{"delta":{"content":"ok"}}]}



data: {"id":"chatcmpl-1","model":"gpt-4o","choices":[],"usage":{"prompt_tokens":5,"completion_tokens":1,"total_tokens":6}}

: comment before done
data: [DONE]

`
	parser.Feed([]byte(rawSSE))
	res := parser.Finalize()

	if !res.SawDone {
		t.Errorf("expected sawDone to be true")
	}
	if res.ExtractedUsage == nil || *res.ExtractedUsage.TotalTokens != 6 {
		t.Errorf("expected total tokens 6, got %+v", res.ExtractedUsage)
	}
}

func TestStreamParser_OversizedEventHandling(t *testing.T) {
	parser := NewStreamParser()

	// 1. Send an oversized event exceeding MaxEventBufferSize (70 KB)
	oversizedLine := "data: " + strings.Repeat("A", 70*1024) + "\n\n"
	parser.Feed([]byte(oversizedLine))

	// 2. Send a valid subsequent event
	validEvent := `data: {"id":"chatcmpl-1","model":"gpt-4o","choices":[],"usage":{"prompt_tokens":20,"completion_tokens":10,"total_tokens":30}}

data: [DONE]

`
	parser.Feed([]byte(validEvent))
	res := parser.Finalize()

	if !res.OversizedEvent {
		t.Errorf("expected OversizedEvent to be true")
	}
	if !res.SawDone {
		t.Errorf("expected sawDone to be true after recovering from oversized event")
	}
	if res.ExtractedUsage == nil || *res.ExtractedUsage.TotalTokens != 30 {
		t.Errorf("expected to recover and parse valid usage event, got %+v", res.ExtractedUsage)
	}
}

func TestStreamParser_CRLFLineEndings(t *testing.T) {
	parser := NewStreamParser()

	rawSSE := "data: {\"id\":\"chatcmpl-1\",\"model\":\"gpt-4o\",\"choices\":[],\"usage\":{\"prompt_tokens\":8,\"completion_tokens\":2,\"total_tokens\":10}}\r\n\r\ndata: [DONE]\r\n\r\n"
	parser.Feed([]byte(rawSSE))
	res := parser.Finalize()

	if !res.SawDone {
		t.Errorf("expected sawDone to be true with CRLF endings")
	}
	if res.ExtractedUsage == nil || *res.ExtractedUsage.TotalTokens != 10 {
		t.Errorf("expected 10 total tokens, got %+v", res.ExtractedUsage)
	}
}

func TestStreamParser_ResidualDataWithoutTrailingNewline(t *testing.T) {
	parser := NewStreamParser()

	// Stream ends abruptly right after data: [DONE] without trailing double newline
	parser.Feed([]byte("data: [DONE]"))
	res := parser.Finalize()

	if !res.SawDone {
		t.Errorf("expected Finalize to process residual data")
	}
}

func BenchmarkStreamParser_FeedChunks(b *testing.B) {
	chunk := []byte(`data: {"id":"chatcmpl-1","model":"gpt-4o-mini","choices":[{"delta":{"content":"token"}}]}\n\n`)
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		p := NewStreamParser()
		for j := 0; j < 50; j++ {
			p.Feed(chunk)
		}
		_ = p.Finalize()
	}
}

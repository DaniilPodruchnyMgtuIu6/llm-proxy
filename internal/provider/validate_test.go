package provider

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestValidateChatBodyRanges(t *testing.T) {
	cases := []struct {
		body    string
		wantErr string
	}{
		{`{"messages":[{"role":"user","content":"hi"}],"top_p":8}`, "top_p"},
		{`{"messages":[{"role":"user","content":"hi"}],"temperature":3}`, "temperature"},
		{`{"messages":[{"role":"user","content":"hi"}],"max_tokens":1}`, "max_tokens"},
		{`{"messages":[{"role":"user","content":"hi"}],"top_k":0}`, "top_k"},
		{`{"messages":[{"role":"user","content":"hi"}],"presence_penalty":5}`, "presence_penalty"},
		{`{"messages":[{"role":"user","content":"hi"}],"frequency_penalty":-3}`, "frequency_penalty"},
		{`{"messages":[{"role":"user","content":"hi"}],"n":2}`, "n must be 1"},
		{`{"messages":[{"role":"user","content":"hi"}],"temperature":0.7,"top_p":1,"max_tokens":64}`, ""},
	}
	for _, tc := range cases {
		err := ValidateChatBody(json.RawMessage(tc.body))
		if tc.wantErr == "" {
			if err != nil {
				t.Fatalf("body=%s err=%v", tc.body, err)
			}
			continue
		}
		if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
			t.Fatalf("body=%s want err containing %q, got %v", tc.body, tc.wantErr, err)
		}
	}
}

func TestIsEmptyLengthCompletion(t *testing.T) {
	empty := json.RawMessage(`{"choices":[{"finish_reason":"length","message":{"role":"assistant"}}]}`)
	if !IsEmptyLengthCompletion(empty) {
		t.Fatal("expected empty length completion")
	}
	ok := json.RawMessage(`{"choices":[{"finish_reason":"stop","message":{"role":"assistant","content":"привет"}}]}`)
	if IsEmptyLengthCompletion(ok) {
		t.Fatal("expected useful completion")
	}
}

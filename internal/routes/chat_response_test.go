package routes

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func runNonStreamFixture(t *testing.T, fixture string) (int, bool, string) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest("POST", "/v1/chat/completions", nil)

	body, err := normalizeMiMoResponseBody(strings.NewReader(fixture))
	if err != nil {
		ctx.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
	} else {
		processNonStream(ctx, body, "fixture-id", "mimo-v2.5", t.Name(), "", "prompt")
	}

	var payload struct {
		Choices []struct {
			Message map[string]json.RawMessage `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		if recorder.Code == http.StatusBadGateway {
			return recorder.Code, false, ""
		}
		t.Fatalf("invalid adapter response JSON: %v", err)
	}
	if len(payload.Choices) != 1 {
		return recorder.Code, false, ""
	}
	raw, present := payload.Choices[0].Message["content"]
	if !present {
		return recorder.Code, false, ""
	}
	var content string
	if err := json.Unmarshal(raw, &content); err != nil {
		t.Fatalf("content is not a string: %v", err)
	}
	return recorder.Code, true, content
}

func TestNonStreamParserFixtures(t *testing.T) {
	fixtures := []struct {
		name        string
		body        string
		wantStatus  int
		wantPresent bool
		wantContent string
	}{
		{
			name:        "MiMo legacy JSON envelope",
			body:        `{"code":0,"msg":"","data":{"result":"OK"}}`,
			wantStatus:  http.StatusOK,
			wantPresent: true,
			wantContent: "OK",
		},
		{
			name:        "named message SSE",
			body:        "event: message\ndata: {\"content\":\"OK\"}\n\n",
			wantStatus:  http.StatusOK,
			wantPresent: true,
			wantContent: "OK",
		},
		{
			name:        "OpenAI data-only SSE chunk",
			body:        "data: {\"id\":\"x\",\"object\":\"chat.completion.chunk\",\"choices\":[{\"index\":0,\"delta\":{\"role\":\"assistant\",\"content\":\"OK\"},\"finish_reason\":null}]}\n\ndata: [DONE]\n\n",
			wantStatus:  http.StatusOK,
			wantPresent: true,
			wantContent: "OK",
		},
		{
			name:        "OpenAI non-stream JSON response",
			body:        `{"choices":[{"message":{"role":"assistant","content":"OK"}}]}`,
			wantStatus:  http.StatusOK,
			wantPresent: true,
			wantContent: "OK",
		},
		{
			name:       "MiMo empty result",
			body:       `{"code":0,"msg":"","data":{"result":"  "}}`,
			wantStatus: http.StatusBadGateway,
		},
		{
			name:       "MiMo business error",
			body:       `{"code":1001,"msg":"upstream error","data":{"result":""}}`,
			wantStatus: http.StatusBadGateway,
		},
		{
			name:       "unsupported plain text response",
			body:       "unexpected upstream text",
			wantStatus: http.StatusBadGateway,
		},
	}

	for _, fixture := range fixtures {
		t.Run(fixture.name, func(t *testing.T) {
			status, present, content := runNonStreamFixture(t, fixture.body)
			if status != fixture.wantStatus {
				t.Fatalf("got HTTP %d, want %d", status, fixture.wantStatus)
			}
			if present != fixture.wantPresent || content != fixture.wantContent {
				t.Fatalf("content present=%t, content=%q; want present=%t, content=%q", present, content, fixture.wantPresent, fixture.wantContent)
			}
		})
	}
}

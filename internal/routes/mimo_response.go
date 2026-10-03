package routes

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"strings"
)

// normalizeMiMoResponseBody converts the legacy MiMo JSON envelope to the
// named message event already consumed by the OpenAI-compatible adapter.
// Non-JSON streams are passed through unchanged so they can be parsed lazily.
func normalizeMiMoResponseBody(body io.Reader) (io.Reader, error) {
	buffered := bufio.NewReaderSize(body, 4096)
	prefix := make([]byte, 0, 16)

	for {
		b, err := buffered.ReadByte()
		if err != nil {
			if errors.Is(err, io.EOF) {
				return nil, errors.New("MiMo returned an empty response")
			}
			return nil, errors.New("could not read MiMo response")
		}
		prefix = append(prefix, b)
		if b != ' ' && b != '\t' && b != '\r' && b != '\n' {
			break
		}
	}

	firstContentByte := prefix[len(prefix)-1]
	if firstContentByte != '{' && firstContentByte != '[' {
		return io.MultiReader(bytes.NewReader(prefix), buffered), nil
	}

	bodyBytes, err := io.ReadAll(io.MultiReader(bytes.NewReader(prefix), buffered))
	if err != nil {
		return nil, errors.New("could not read MiMo JSON response")
	}

	content, err := extractMiMoJSONContent(bodyBytes)
	if err != nil {
		return nil, err
	}

	eventData, err := json.Marshal(struct {
		Content string `json:"content"`
	}{Content: content})
	if err != nil {
		return nil, errors.New("could not encode MiMo response")
	}

	normalized := make([]byte, 0, len(eventData)+32)
	normalized = append(normalized, "event: message\ndata: "...)
	normalized = append(normalized, eventData...)
	normalized = append(normalized, '\n', '\n')
	return bytes.NewReader(normalized), nil
}

func extractMiMoJSONContent(body []byte) (string, error) {
	var envelope struct {
		Code *int `json:"code"`
		Data struct {
			Result *string `json:"result"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil {
		return "", errors.New("MiMo returned an invalid JSON response")
	}
	if envelope.Data.Result != nil {
		if envelope.Code != nil && *envelope.Code != 0 {
			return "", errors.New("MiMo reported an upstream error")
		}
		if strings.TrimSpace(*envelope.Data.Result) == "" {
			return "", errors.New("MiMo returned an empty completion")
		}
		return *envelope.Data.Result, nil
	}

	var openAIResponse struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
			Delta struct {
				Content string `json:"content"`
			} `json:"delta"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(body, &openAIResponse); err == nil && len(openAIResponse.Choices) > 0 {
		content := openAIResponse.Choices[0].Message.Content
		if content == "" {
			content = openAIResponse.Choices[0].Delta.Content
		}
		if strings.TrimSpace(content) == "" {
			return "", errors.New("MiMo returned an empty completion")
		}
		return content, nil
	}

	return "", errors.New("MiMo returned an unsupported JSON response")
}

package narrator

import (
    "os"
    "path/filepath"
    "strings"

    "mimoproxy/internal/models"
)

const defaultPromptPath = "prompts/dnd_narrator.md"

// ApplySystemPrompt injects the narrator policy only when DND_NARRATOR_MODE is enabled.
// An explicitly supplied system message is preserved after the narrator policy.
func ApplySystemPrompt(messages []models.Message) []models.Message {
    enabled := strings.EqualFold(strings.TrimSpace(os.Getenv("DND_NARRATOR_MODE")), "true")
    if !enabled {
        return messages
    }

    prompt := strings.TrimSpace(os.Getenv("DND_NARRATOR_SYSTEM_PROMPT"))
    if prompt == "" {
        path := os.Getenv("DND_NARRATOR_PROMPT_FILE")
        if path == "" {
            path = defaultPromptPath
        }
        if data, err := os.ReadFile(filepath.Clean(path)); err == nil {
            prompt = strings.TrimSpace(string(data))
        }
    }
    if prompt == "" {
        return messages
    }

    out := make([]models.Message, 0, len(messages)+1)
    inserted := false
    for _, message := range messages {
        if message.Role == "system" {
            if !inserted {
                out = append(out, models.Message{Role: "system", Content: prompt})
                inserted = true
            }
            if existing := strings.TrimSpace(extractText(message.Content)); existing != "" {
                out = append(out, models.Message{Role: "system", Content: existing})
            }
            continue
        }
        out = append(out, message)
    }
    if !inserted {
        out = append([]models.Message{{Role: "system", Content: prompt}}, out...)
    }
    return out
}

func extractText(content interface{}) string {
    if text, ok := content.(string); ok {
        return text
    }
    if parts, ok := content.([]interface{}); ok {
        var out []string
        for _, raw := range parts {
            if part, ok := raw.(map[string]interface{}); ok {
                if text, ok := part["text"].(string); ok {
                    out = append(out, text)
                }
            }
        }
        return strings.Join(out, "\n")
    }
    return ""
}

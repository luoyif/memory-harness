package connectors

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
)

const ParserVersion = "conversation-json-v1"

type Message struct {
	ExternalID string   `json:"external_message_id"`
	ParentID   string   `json:"parent_id,omitempty"`
	Role       string   `json:"role"`
	ObservedAt string   `json:"observed_at,omitempty"`
	Text       string   `json:"text"`
	Warnings   []string `json:"warnings,omitempty"`
}

type Conversation struct {
	ExternalID string    `json:"external_conversation_id"`
	Title      string    `json:"title"`
	CreatedAt  string    `json:"created_at,omitempty"`
	UpdatedAt  string    `json:"updated_at,omitempty"`
	Messages   []Message `json:"messages"`
}

type Preview struct {
	Format        string `json:"format"`
	Conversations int    `json:"conversations"`
	Messages      int    `json:"messages"`
	DateFrom      string `json:"date_from,omitempty"`
	DateTo        string `json:"date_to,omitempty"`
	Warnings      int    `json:"warnings"`
}

func Parse(format string, payload []byte) ([]Conversation, Preview, error) {
	format = strings.ToLower(strings.TrimSpace(format))
	var conversations []Conversation
	var err error
	switch format {
	case "normalized":
		err = decodeJSON(payload, &conversations)
	case "chatgpt":
		conversations, err = parseChatGPT(payload)
	case "claude":
		conversations, err = parseClaude(payload)
	case "deepseek":
		conversations, err = parseDeepSeek(payload)
	default:
		return nil, Preview{}, fmt.Errorf("unsupported conversation format %q", format)
	}
	if err != nil {
		return nil, Preview{}, err
	}
	if len(conversations) == 0 || len(conversations) > 1000 {
		return nil, Preview{}, errors.New("conversation export must contain between 1 and 1000 conversations")
	}
	preview := Preview{Format: format, Conversations: len(conversations)}
	for i := range conversations {
		conversation := &conversations[i]
		conversation.ExternalID = strings.TrimSpace(conversation.ExternalID)
		conversation.Title = strings.TrimSpace(conversation.Title)
		if conversation.ExternalID == "" {
			return nil, Preview{}, fmt.Errorf("conversation %d has no id", i)
		}
		if conversation.Title == "" {
			conversation.Title = "未命名会话"
		}
		if len(conversation.Messages) == 0 || len(conversation.Messages) > 5000 {
			return nil, Preview{}, fmt.Errorf("conversation %s must contain between 1 and 5000 messages", conversation.ExternalID)
		}
		for j := range conversation.Messages {
			message := &conversation.Messages[j]
			message.ExternalID = strings.TrimSpace(message.ExternalID)
			message.Role = normalizeRole(message.Role)
			message.Text = strings.TrimSpace(message.Text)
			if message.ExternalID == "" {
				message.ExternalID = fmt.Sprintf("message-%06d", j+1)
			}
			if message.Text == "" || len(message.Text) > 2<<20 {
				return nil, Preview{}, fmt.Errorf("conversation %s message %s has invalid text size", conversation.ExternalID, message.ExternalID)
			}
			if message.ObservedAt != "" {
				parsed, err := parseTime(message.ObservedAt)
				if err != nil {
					return nil, Preview{}, fmt.Errorf("conversation %s message %s: %w", conversation.ExternalID, message.ExternalID, err)
				}
				message.ObservedAt = parsed
				if preview.DateFrom == "" || parsed < preview.DateFrom {
					preview.DateFrom = parsed
				}
				if preview.DateTo == "" || parsed > preview.DateTo {
					preview.DateTo = parsed
				}
			}
			preview.Messages++
			preview.Warnings += len(message.Warnings)
		}
	}
	return conversations, preview, nil
}

func decodeJSON(payload []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.UseNumber()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	if decoder.More() {
		return errors.New("multiple JSON values are not supported")
	}
	return nil
}

func normalizeRole(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "human", "user":
		return "user"
	case "assistant", "ai":
		return "assistant"
	case "system":
		return "system"
	case "tool", "function":
		return "tool"
	default:
		return "unknown"
	}
}

func parseTime(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", nil
	}
	if parsed, err := time.Parse(time.RFC3339, value); err == nil {
		return parsed.UTC().Format(time.RFC3339Nano), nil
	}
	return "", fmt.Errorf("invalid timestamp %q", value)
}

func epoch(value float64) string {
	if value <= 0 {
		return ""
	}
	seconds, nanos := int64(value), int64((value-float64(int64(value)))*1e9)
	return time.Unix(seconds, nanos).UTC().Format(time.RFC3339Nano)
}

type chatGPTConversation struct {
	ID          string                 `json:"id"`
	Title       string                 `json:"title"`
	CreateTime  float64                `json:"create_time"`
	UpdateTime  float64                `json:"update_time"`
	CurrentNode string                 `json:"current_node"`
	Mapping     map[string]chatGPTNode `json:"mapping"`
}

type chatGPTNode struct {
	ID      string          `json:"id"`
	Parent  string          `json:"parent"`
	Message *chatGPTMessage `json:"message"`
}

type chatGPTMessage struct {
	ID         string   `json:"id"`
	CreateTime float64  `json:"create_time"`
	Weight     *float64 `json:"weight"`
	Author     struct {
		Role string `json:"role"`
	} `json:"author"`
	Content struct {
		ContentType string `json:"content_type"`
		Parts       []any  `json:"parts"`
	} `json:"content"`
}

func parseChatGPT(payload []byte) ([]Conversation, error) {
	var raw []chatGPTConversation
	if err := decodeJSON(payload, &raw); err != nil {
		return nil, err
	}
	out := make([]Conversation, 0, len(raw))
	for _, conversation := range raw {
		path := []chatGPTNode{}
		seen := map[string]bool{}
		current := conversation.CurrentNode
		for current != "" && !seen[current] {
			seen[current] = true
			node, ok := conversation.Mapping[current]
			if !ok {
				break
			}
			path = append(path, node)
			current = node.Parent
		}
		for left, right := 0, len(path)-1; left < right; left, right = left+1, right-1 {
			path[left], path[right] = path[right], path[left]
		}
		item := Conversation{ExternalID: conversation.ID, Title: conversation.Title, CreatedAt: epoch(conversation.CreateTime), UpdatedAt: epoch(conversation.UpdateTime)}
		for _, node := range path {
			message := node.Message
			if message == nil || message.Author.Role == "system" || (message.Weight != nil && *message.Weight == 0) || (message.Content.ContentType != "text" && message.Content.ContentType != "multimodal_text") {
				continue
			}
			parts := []string{}
			for _, part := range message.Content.Parts {
				if text, ok := part.(string); ok && strings.TrimSpace(text) != "" {
					parts = append(parts, text)
				}
			}
			if len(parts) == 0 {
				continue
			}
			id := message.ID
			if id == "" {
				id = node.ID
			}
			item.Messages = append(item.Messages, Message{ExternalID: id, ParentID: node.Parent, Role: message.Author.Role, ObservedAt: epoch(message.CreateTime), Text: strings.Join(parts, "\n")})
		}
		if len(item.Messages) > 0 {
			out = append(out, item)
		}
	}
	return out, nil
}

type claudeConversation struct {
	UUID         string          `json:"uuid"`
	Name         string          `json:"name"`
	CreatedAt    string          `json:"created_at"`
	UpdatedAt    string          `json:"updated_at"`
	ChatMessages []claudeMessage `json:"chat_messages"`
}

type claudeMessage struct {
	UUID      string `json:"uuid"`
	Sender    string `json:"sender"`
	CreatedAt string `json:"created_at"`
	Text      string `json:"text"`
	Content   []struct {
		Type     string `json:"type"`
		Text     string `json:"text"`
		Thinking string `json:"thinking"`
	} `json:"content"`
}

func parseClaude(payload []byte) ([]Conversation, error) {
	var raw []claudeConversation
	if err := decodeJSON(payload, &raw); err != nil {
		return nil, err
	}
	out := make([]Conversation, 0, len(raw))
	for _, conversation := range raw {
		item := Conversation{ExternalID: conversation.UUID, Title: conversation.Name, CreatedAt: conversation.CreatedAt, UpdatedAt: conversation.UpdatedAt}
		for _, rawMessage := range conversation.ChatMessages {
			parts := []string{}
			warnings := []string{}
			for _, block := range rawMessage.Content {
				if block.Type == "text" && strings.TrimSpace(block.Text) != "" {
					parts = append(parts, block.Text)
				} else if block.Type == "thinking" && strings.TrimSpace(block.Thinking) != "" {
					warnings = append(warnings, "thinking_excluded_from_default_distillation_preserved_in_raw")
				}
			}
			if len(parts) == 0 && strings.TrimSpace(rawMessage.Text) != "" {
				parts = append(parts, rawMessage.Text)
			}
			if len(parts) == 0 {
				continue
			}
			item.Messages = append(item.Messages, Message{ExternalID: rawMessage.UUID, Role: rawMessage.Sender, ObservedAt: rawMessage.CreatedAt, Text: strings.Join(parts, "\n"), Warnings: warnings})
		}
		if len(item.Messages) > 0 {
			out = append(out, item)
		}
	}
	return out, nil
}

type deepSeekConversation struct {
	ID         string                  `json:"id"`
	Title      string                  `json:"title"`
	InsertedAt string                  `json:"inserted_at"`
	UpdatedAt  string                  `json:"updated_at"`
	Mapping    map[string]deepSeekNode `json:"mapping"`
}

type deepSeekNode struct {
	ID       string           `json:"id"`
	Children []string         `json:"children"`
	Message  *deepSeekMessage `json:"message"`
}

type deepSeekMessage struct {
	InsertedAt string `json:"inserted_at"`
	Fragments  []struct {
		Type    string `json:"type"`
		Content string `json:"content"`
		URL     string `json:"url"`
		Results []struct {
			URL     string `json:"url"`
			Title   string `json:"title"`
			Snippet string `json:"snippet"`
		} `json:"results"`
	} `json:"fragments"`
}

func parseDeepSeek(payload []byte) ([]Conversation, error) {
	var raw []deepSeekConversation
	if err := decodeJSON(payload, &raw); err != nil {
		return nil, err
	}
	out := make([]Conversation, 0, len(raw))
	for _, conversation := range raw {
		item := Conversation{ExternalID: conversation.ID, Title: conversation.Title, CreatedAt: conversation.InsertedAt, UpdatedAt: conversation.UpdatedAt}
		current := "root"
		seen := map[string]bool{}
		parent := ""
		for current != "" && !seen[current] {
			seen[current] = true
			node, ok := conversation.Mapping[current]
			if !ok {
				break
			}
			if node.Message != nil {
				parts, warnings := []string{}, []string{}
				role := "assistant"
				for _, fragment := range node.Message.Fragments {
					switch fragment.Type {
					case "REQUEST":
						role = "user"
						parts = append(parts, fragment.Content)
					case "RESPONSE":
						parts = append(parts, fragment.Content)
					case "SEARCH":
						for _, result := range fragment.Results {
							parts = append(parts, strings.TrimSpace(result.Title+"\n"+result.Snippet+"\n"+result.URL))
						}
					case "READ_LINK":
						parts = append(parts, fragment.URL)
					case "THINK":
						warnings = append(warnings, "thinking_excluded_from_default_distillation_preserved_in_raw")
					}
				}
				text := strings.TrimSpace(strings.Join(parts, "\n\n"))
				if text != "" {
					item.Messages = append(item.Messages, Message{ExternalID: node.ID, ParentID: parent, Role: role, ObservedAt: node.Message.InsertedAt, Text: text, Warnings: warnings})
					parent = node.ID
				}
			}
			if len(node.Children) == 0 {
				break
			}
			current = node.Children[0]
		}
		if len(item.Messages) > 0 {
			out = append(out, item)
		}
	}
	return out, nil
}

func SortMessages(messages []Message) {
	sort.SliceStable(messages, func(i, j int) bool {
		if messages[i].ObservedAt == messages[j].ObservedAt {
			return i < j
		}
		return messages[i].ObservedAt < messages[j].ObservedAt
	})
}

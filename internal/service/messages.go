package service

import (
	"encoding/json"
	"strings"
	"time"

	"traffic-go/internal/domain"
)

func NormalizeMessageFrom(v string) string {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "", "system":
		return domain.RoleSystem
	case "user", "engineer", "human":
		return domain.RoleUser
	case "assistant", "ai", "ai_assistant", "engineer_ai":
		return domain.RoleAssistant
	case "captain", "role_soc_captain", "soc_captain", "_captain":
		return domain.RoleCaptain
	case "manager", "role_soc_manager", "soc_manager", "_manager":
		return domain.RoleManager
	case "operator", "role_soc_operator", "soc_operator", "_operator":
		return domain.RoleOperator
	case "executor", "role_soc_executor", "soc_executor", "_executor":
		return domain.RoleExecutor
	case "expert", "role_soc_expert", "soc_expert", "_expert":
		return domain.RoleExpert
	default:
		return strings.TrimSpace(v)
	}
}

func SenderType(messageFrom string) string {
	switch NormalizeMessageFrom(messageFrom) {
	case domain.RoleUser:
		return "user"
	case domain.RoleAssistant:
		return "ai"
	case domain.RoleSystem:
		return "system"
	default:
		return "agent"
	}
}

func StandardContent(data any) string {
	payload := map[string]any{
		"timestamp": time.Now().UTC().Format(time.RFC3339Nano),
		"data":      data,
	}
	b, _ := json.Marshal(payload)
	return string(b)
}

func responseText(text string, extra map[string]any) map[string]any {
	out := map[string]any{"response_text": text}
	for k, v := range extra {
		out[k] = v
	}
	return out
}

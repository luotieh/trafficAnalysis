package httpapi

import (
	"encoding/json"
	"fmt"
	"strings"

	"traffic-go/internal/domain"
)

func (s *Server) engineerEventPrompt(event domain.Event, question string) string {
	question = cleanEngineerQuestion(question)
	var b strings.Builder
	b.WriteString("# 安全事件完整信息\n\n")
	b.WriteString(fmt.Sprintf("事件ID: %s\n", event.EventID))
	b.WriteString(fmt.Sprintf("事件名称: %s\n", firstNonEmpty(event.EventName, event.Title, "未命名事件")))
	b.WriteString(fmt.Sprintf("事件描述: %s\n", firstNonEmpty(event.Message, "无描述")))
	b.WriteString(fmt.Sprintf("事件上下文: %s\n", firstNonEmpty(event.Context, "无上下文")))
	b.WriteString(fmt.Sprintf("严重程度: %s\n", firstNonEmpty(event.Severity, "未知")))
	b.WriteString(fmt.Sprintf("事件来源: %s\n", firstNonEmpty(event.Source, "未知")))
	b.WriteString(fmt.Sprintf("事件状态: %s\n", firstNonEmpty(event.EventStatus, "未知")))
	b.WriteString(fmt.Sprintf("当前轮次: %d\n", event.CurrentRound))
	b.WriteString(fmt.Sprintf("创建时间: %s\n\n", event.CreatedAt.Format("2006-01-02 15:04:05")))

	if len(event.Observables) > 0 {
		b.WriteString("## 可观察对象\n")
		for _, ioc := range event.Observables {
			b.WriteString(fmt.Sprintf("- type=%s value=%s role=%s\n", ioc.Type, ioc.Value, ioc.Role))
		}
		b.WriteString("\n")
	}

	writeJSONSection(&b, "## 自动驾驶任务", s.services.Store.ListTasks(event.EventID))
	writeJSONSection(&b, "## 自动驾驶动作", s.services.Store.ListActions(event.EventID))
	writeJSONSection(&b, "## 自动驾驶命令", s.services.Store.ListCommands(event.EventID))
	writeJSONSection(&b, "## 执行结果", s.services.Store.ListExecutions(event.EventID))
	writeJSONSection(&b, "## 事件总结", s.services.Store.ListSummaries(event.EventID))
	writeEngineerHistory(&b, s.services.Store.ListMessages(event.EventID))

	b.WriteString("# 当前工程师问题\n")
	b.WriteString(question)
	b.WriteString("\n\n")
	b.WriteString(`# 回答要求
你是专业的安全运营AI助手。必须基于上面的事件完整信息回答，不要要求用户再次提供事件详情。
如果用户要求“分析该事件”或“形成报告”，请直接生成报告，至少包含：
1. 事件概览
2. 关键证据
3. 攻击源与受影响资产分析
4. 已执行处置/自动驾驶进展
5. 风险判断
6. 后续处置建议
7. 可交付给安全团队的结论
如果现有信息不足，请明确列出缺口，但仍要基于已有信息完成初步分析。`)
	return b.String()
}

func cleanEngineerQuestion(question string) string {
	question = strings.TrimSpace(question)
	question = strings.TrimPrefix(question, "@AI")
	question = strings.TrimPrefix(question, "@ai")
	return strings.TrimSpace(question)
}

func writeJSONSection(b *strings.Builder, title string, v any) {
	b.WriteString(title)
	b.WriteString("\n")
	raw, _ := json.MarshalIndent(v, "", "  ")
	if string(raw) == "null" || string(raw) == "[]" {
		b.WriteString("暂无\n\n")
		return
	}
	b.Write(raw)
	b.WriteString("\n\n")
}

func writeEngineerHistory(b *strings.Builder, messages []domain.Message) {
	lines := []string{}
	for _, msg := range messages {
		if msg.MessageCategory != "engineer_chat" {
			continue
		}
		role := ""
		switch msg.SenderType {
		case "user":
			role = "工程师"
		case "ai":
			role = "AI助手"
		default:
			continue
		}
		content := strings.TrimSpace(msg.MessageContent)
		if content == "" {
			continue
		}
		lines = append(lines, fmt.Sprintf("%s: %s", role, content))
	}
	if len(lines) > 20 {
		lines = lines[len(lines)-20:]
	}
	b.WriteString("## 最近工程师对话\n")
	if len(lines) == 0 {
		b.WriteString("暂无历史对话\n\n")
		return
	}
	for _, line := range lines {
		b.WriteString("- ")
		b.WriteString(line)
		b.WriteString("\n")
	}
	b.WriteString("\n")
}

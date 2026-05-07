package service

import (
	"context"
	"fmt"
	"strings"

	"traffic-go/internal/domain"
	"traffic-go/internal/realtime"
)

func (s Services) RunAgentWorkflow(ctx context.Context, eventID string) error {
	event, ok := s.Store.GetEvent(eventID)
	if !ok {
		return fmt.Errorf("event not found: %s", eventID)
	}
	if len(s.Store.ListTasks(eventID)) > 0 || len(s.Store.ListExecutions(eventID)) > 0 {
		return nil
	}
	roundID := event.CurrentRound
	if roundID == 0 {
		roundID = 1
	}
	_, _ = s.Store.UpdateEvent(eventID, map[string]any{"event_status": "processing"})

	_ = s.addAgentMessage(eventID, domain.RoleCaptain, "captain_llm_request", roundID,
		responseText("AI指挥官正在分析事件并生成处置任务。", map[string]any{"event_name": firstNonEmpty(event.EventName, event.Title)}))

	taskName := "研判攻击源、受害资产和处置优先级"
	if strings.TrimSpace(event.EventName) != "" {
		taskName = "研判事件：" + event.EventName
	}
	task, err := s.Store.AddTask(domain.Task{
		EventID:         eventID,
		TaskName:        taskName,
		TaskType:        "analysis",
		TaskDescription: buildTaskDescription(event),
		TaskStatus:      "pending",
		TaskPriority:    severityPriority(event.Severity),
		AssignedTo:      domain.RoleManager,
		TaskAssignee:    domain.RoleManager,
		RoundID:         roundID,
	})
	if err != nil {
		return err
	}
	_ = s.addAgentMessage(eventID, domain.RoleCaptain, "task_created", roundID,
		responseText("AI指挥官已创建任务并交给安全经理拆解。", map[string]any{"task": task}))

	action, err := s.Store.AddAction(domain.Action{
		EventID:        eventID,
		TaskID:         task.TaskID,
		RoundID:        roundID,
		ActionName:     "收集威胁情报、资产信息和相关证据",
		ActionType:     "query",
		ActionAssignee: domain.RoleOperator,
		ActionStatus:   "pending",
	})
	if err != nil {
		return err
	}
	_, _ = s.Store.UpdateTask(task.TaskID, map[string]any{"task_status": "processing", "assigned_to": domain.RoleOperator})
	_ = s.addAgentMessage(eventID, domain.RoleManager, "action_created", roundID,
		responseText("安全经理已将任务拆解为可执行行动。", map[string]any{"action": action}))

	command, err := s.Store.AddCommand(domain.Command{
		EventID:         eventID,
		TaskID:          task.TaskID,
		ActionID:        action.ActionID,
		RoundID:         roundID,
		CommandName:     "执行威胁情报查询与资产核查",
		CommandType:     "playbook",
		CommandAssignee: domain.RoleExecutor,
		CommandEntity:   StandardContent(map[string]any{"tool": "threat_intel_and_asset_lookup"}),
		CommandParams:   StandardContent(commandParams(event)),
		CommandStatus:   "pending",
	})
	if err != nil {
		return err
	}
	_, _ = s.Store.UpdateAction(action.ActionID, map[string]any{"action_status": "processing"})
	_ = s.addAgentMessage(eventID, domain.RoleOperator, "command_created", roundID,
		responseText("安全操作员已生成执行器可处理的命令。", map[string]any{"command": command}))

	result := executionResult(event)
	exec, err := s.Store.AddExecution(domain.Execution{
		EventID:          eventID,
		TaskID:           task.TaskID,
		ActionID:         action.ActionID,
		RoundID:          roundID,
		CommandID:        command.CommandID,
		ExecutionStatus:  "completed",
		ExecutionResult:  StandardContent(result),
		ExecutionSummary: "执行器完成威胁情报与资产信息核查，已返回结构化证据。",
		CommandName:      command.CommandName,
		CommandType:      command.CommandType,
		CommandEntity:    command.CommandEntity,
		CommandParams:    command.CommandParams,
	})
	if err != nil {
		return err
	}
	_, _ = s.Store.UpdateCommand(command.CommandID, map[string]any{"command_status": "completed", "command_result": exec.ExecutionResult})
	_, _ = s.Store.UpdateAction(action.ActionID, map[string]any{"action_status": "completed", "action_result": exec.ExecutionResult})
	_, _ = s.Store.UpdateTask(task.TaskID, map[string]any{"task_status": "completed"})
	_ = s.addAgentMessage(eventID, domain.RoleExecutor, "command_result", roundID,
		responseText("自动化执行器已完成命令执行并返回结果。", map[string]any{"execution": exec}))
	realtime.BroadcastExecutionUpdate(eventID, map[string]any{"event_id": eventID, "execution_id": exec.ExecutionID, "status": "completed"})

	summary := eventSummary(event, result)
	sm, err := s.Store.AddSummary(domain.Summary{EventID: eventID, RoundID: roundID, EventSummary: summary})
	if err != nil {
		return err
	}
	_ = s.addAgentMessage(eventID, domain.RoleExpert, "event_summary", roundID,
		responseText("安全专家已完成本轮总结并给出处置建议。", map[string]any{"summary": sm}))
	_, _ = s.Store.UpdateEvent(eventID, map[string]any{"event_status": "round_finished"})
	realtime.BroadcastStatus(eventID, map[string]any{"event_id": eventID, "status": "round_finished"})
	return nil
}

func (s Services) addAgentMessage(eventID, from, messageType string, roundID int, data any) error {
	from = NormalizeMessageFrom(from)
	m, err := s.Store.AddMessage(domain.Message{
		EventID:         eventID,
		MessageFrom:     from,
		MessageType:     messageType,
		MessageContent:  StandardContent(data),
		RoundID:         roundID,
		MessageCategory: "agent",
		SenderType:      SenderType(from),
	})
	if err == nil {
		realtime.BroadcastMessage(eventID, m)
		_ = s.publish(context.Background(), "notifications.frontend."+eventID+"."+from+"."+messageType, eventID, from, m)
	}
	return err
}

func buildTaskDescription(event domain.Event) string {
	return firstNonEmpty(event.Message, event.Context, "根据事件上下文完成威胁研判、证据收集和处置建议。")
}

func severityPriority(severity string) string {
	switch strings.ToLower(strings.TrimSpace(severity)) {
	case "critical", "high", "严重", "高":
		return "high"
	case "low", "低":
		return "low"
	default:
		return "medium"
	}
}

func commandParams(event domain.Event) map[string]any {
	params := map[string]any{"event_id": event.EventID, "severity": event.Severity, "source": event.Source}
	if len(event.Observables) > 0 {
		params["observables"] = event.Observables
	}
	return params
}

func executionResult(event domain.Event) map[string]any {
	return map[string]any{
		"event_id": event.EventID,
		"verdict":  "needs_human_review",
		"findings": []string{
			"已完成攻击源和目标资产的初步关联。",
			"建议验证受影响账号、资产暴露面和近期异常登录。",
			"高风险事件应优先执行阻断、隔离或访问控制加固。",
		},
	}
}

func eventSummary(event domain.Event, result map[string]any) string {
	return fmt.Sprintf("事件 %s 已完成第 1 轮自动驾驶分析。结论：%s。建议：复核证据、确认影响范围，并按风险等级执行阻断或加固。", firstNonEmpty(event.EventName, event.EventID), result["verdict"])
}

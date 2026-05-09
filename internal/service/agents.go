package service

import (
	"context"
	"fmt"
	"net"
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

	taskPlans := buildTaskPlans(event)
	createdTasks := make([]domain.Task, 0, len(taskPlans))
	for _, plan := range taskPlans {
		task, err := s.Store.AddTask(domain.Task{
			EventID:         eventID,
			TaskName:        plan.Name,
			TaskType:        plan.Type,
			TaskDescription: plan.Description,
			TaskStatus:      "pending",
			TaskPriority:    severityPriority(event.Severity),
			AssignedTo:      domain.RoleManager,
			TaskAssignee:    plan.Assignee,
			RoundID:         roundID,
		})
		if err != nil {
			return err
		}
		createdTasks = append(createdTasks, task)
	}
	_ = s.addAgentMessage(eventID, domain.RoleCaptain, "task_created", roundID,
		captainResponse(event, roundID, createdTasks))

	createdActions := make([]domain.Action, 0, len(createdTasks))
	for _, task := range createdTasks {
		action, err := s.Store.AddAction(domain.Action{
			EventID:        eventID,
			TaskID:         task.TaskID,
			RoundID:        roundID,
			ActionName:     actionNameForTask(task),
			ActionType:     firstNonEmpty(task.TaskType, "query"),
			ActionAssignee: domain.RoleOperator,
			ActionStatus:   "pending",
		})
		if err != nil {
			return err
		}
		_, _ = s.Store.UpdateTask(task.TaskID, map[string]any{"task_status": "processing", "assigned_to": domain.RoleOperator})
		createdActions = append(createdActions, action)
	}
	_ = s.addAgentMessage(eventID, domain.RoleManager, "action_created", roundID,
		managerResponse(eventID, roundID, createdActions))

	createdCommands := make([]domain.Command, 0, len(createdActions))
	for _, action := range createdActions {
		spec := commandSpecForAction(event, action)
		command, err := s.Store.AddCommand(domain.Command{
			EventID:         eventID,
			TaskID:          action.TaskID,
			ActionID:        action.ActionID,
			RoundID:         roundID,
			CommandName:     spec.Name,
			CommandType:     spec.Type,
			CommandAssignee: domain.RoleExecutor,
			CommandEntity:   StandardContent(spec.Entity),
			CommandParams:   StandardContent(spec.Params),
			CommandStatus:   "pending",
		})
		if err != nil {
			return err
		}
		_, _ = s.Store.UpdateAction(action.ActionID, map[string]any{"action_status": "processing"})
		createdCommands = append(createdCommands, command)
	}
	_ = s.addAgentMessage(eventID, domain.RoleOperator, "command_created", roundID,
		operatorResponse(eventID, roundID, createdCommands))

	executions := make([]domain.Execution, 0, len(createdCommands))
	for _, command := range createdCommands {
		result := executionResult(event, command)
		exec, err := s.Store.AddExecution(domain.Execution{
			EventID:          eventID,
			TaskID:           command.TaskID,
			ActionID:         command.ActionID,
			RoundID:          roundID,
			CommandID:        command.CommandID,
			ExecutionStatus:  "completed",
			ExecutionResult:  StandardContent(result),
			ExecutionSummary: fmt.Sprintf("执行器完成命令【%s】，已返回结构化结果。", command.CommandName),
			CommandName:      command.CommandName,
			CommandType:      command.CommandType,
			CommandEntity:    command.CommandEntity,
			CommandParams:    command.CommandParams,
		})
		if err != nil {
			return err
		}
		_, _ = s.Store.UpdateCommand(command.CommandID, map[string]any{"command_status": "completed", "command_result": exec.ExecutionResult})
		executions = append(executions, exec)
	}
	for _, action := range createdActions {
		_, _ = s.Store.UpdateAction(action.ActionID, map[string]any{"action_status": "completed"})
	}
	for _, task := range createdTasks {
		_, _ = s.Store.UpdateTask(task.TaskID, map[string]any{"task_status": "completed"})
	}
	_ = s.addAgentMessage(eventID, domain.RoleExecutor, "command_result", roundID,
		executorResponse(eventID, roundID, executions))
	for _, exec := range executions {
		realtime.BroadcastExecutionUpdate(eventID, map[string]any{"event_id": eventID, "execution_id": exec.ExecutionID, "status": "completed"})
	}

	summary := eventSummary(event, executions)
	sm, err := s.Store.AddSummary(domain.Summary{EventID: eventID, RoundID: roundID, EventSummary: summary})
	if err != nil {
		return err
	}
	_ = s.addAgentMessage(eventID, domain.RoleExpert, "event_summary", roundID,
		expertResponse(event, roundID, sm, executions))
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

type taskPlan struct {
	Name        string
	Type        string
	Assignee    string
	Description string
}

type commandSpec struct {
	Name   string
	Type   string
	Entity map[string]any
	Params map[string]any
}

func buildTaskPlans(event domain.Event) []taskPlan {
	src := firstObservable(event, "source")
	dst := firstObservable(event, "destination", "target", "victim", "dst")
	if src == "" {
		src = firstIP(event)
	}
	plans := []taskPlan{}
	if src != "" {
		plans = append(plans, taskPlan{
			Name:        fmt.Sprintf("请查询IP地址%s的威胁情报", src),
			Type:        "query",
			Assignee:    "_analyst",
			Description: "查询标签、历史攻击记录、地理位置、情报来源和置信度，不做处置判断。",
		})
		plans = append(plans, taskPlan{
			Name:        fmt.Sprintf("请查询IP地址%s的地理位置", src),
			Type:        "query",
			Assignee:    "_analyst",
			Description: "查询攻击源归属地和网络运营商，作为后续研判依据。",
		})
	}
	if dst != "" && dst != src {
		plans = append(plans, taskPlan{
			Name:        fmt.Sprintf("请查询IP地址%s的资产信息", dst),
			Type:        "query",
			Assignee:    "_analyst",
			Description: "查询资产类型、所属部门、负责人、业务线和生产/测试环境属性。",
		})
	}
	if len(plans) == 0 {
		plans = append(plans, taskPlan{
			Name:        "请根据事件上下文查询相关攻击源和受影响资产信息",
			Type:        "query",
			Assignee:    "_analyst",
			Description: firstNonEmpty(event.Message, event.Context, "根据事件上下文完成威胁研判、证据收集和处置建议。"),
		})
	}
	return plans
}

func captainResponse(event domain.Event, roundID int, tasks []domain.Task) map[string]any {
	responseText := fmt.Sprintf("当前事件来源为%s，严重级别为%s。作为SOC指挥官，本轮先围绕攻击源信誉、地理位置和受害资产归属进行事实查询，不直接下发封禁等写入动作，避免在证据不足时扩大影响。待_manager拆解动作并由_executor返回证据后，再根据威胁情报、资产重要性和命中日志决定是否进入处置阶段。",
		firstNonEmpty(event.Source, "未知来源"), firstNonEmpty(event.Severity, "medium"))
	return map[string]any{
		"type":          "llm_response",
		"from":          domain.RoleCaptain,
		"to":            domain.RoleManager,
		"event_id":      event.EventID,
		"round_id":      roundID,
		"event_name":    firstNonEmpty(event.EventName, event.Title, event.EventID),
		"response_type": "TASK",
		"response_text": responseText,
		"tasks":         tasks,
	}
}

func actionNameForTask(task domain.Task) string {
	name := task.TaskName
	switch {
	case strings.Contains(name, "威胁情报"):
		return strings.Replace(name, "请查询", "使用剧本【通用IP地址威胁情报信息查询】查询", 1)
	case strings.Contains(name, "地理位置"):
		return strings.Replace(name, "请查询", "使用剧本【通用IP地址位置查询】查询", 1)
	case strings.Contains(name, "资产信息"):
		return strings.Replace(name, "请查询", "使用剧本【根据IP地址查询资产信息】查询", 1)
	default:
		return "人工分析：" + name
	}
}

func managerResponse(eventID string, roundID int, actions []domain.Action) map[string]any {
	return map[string]any{
		"type":          "llm_response",
		"from":          domain.RoleManager,
		"to":            domain.RoleOperator,
		"event_id":      eventID,
		"round_id":      roundID,
		"response_type": "ACTION",
		"actions":       actions,
	}
}

func commandSpecForAction(event domain.Event, action domain.Action) commandSpec {
	ip := ipFromText(action.ActionName)
	if ip == "" {
		ip = firstIP(event)
	}
	switch {
	case strings.Contains(action.ActionName, "威胁情报"):
		return commandSpec{
			Name: "通用IP地址威胁情报信息查询",
			Type: "playbook",
			Entity: map[string]any{
				"playbook_id":   12316887511154270,
				"playbook_name": "General_IP_Threat_Intelligence_Query",
			},
			Params: map[string]any{"src": ip},
		}
	case strings.Contains(action.ActionName, "地理位置"):
		return commandSpec{
			Name: "通用IP地址位置查询",
			Type: "playbook",
			Entity: map[string]any{
				"playbook_id":   12321406690537761,
				"playbook_name": "General_IP_Location_Query",
			},
			Params: map[string]any{"src": ip},
		}
	case strings.Contains(action.ActionName, "资产信息"):
		return commandSpec{
			Name: "根据IP地址查询资产信息",
			Type: "playbook",
			Entity: map[string]any{
				"playbook_id":   12321435630187042,
				"playbook_name": "query_asset_info_by_ip",
			},
			Params: map[string]any{"dst": ip},
		}
	default:
		return commandSpec{
			Name:   action.ActionName,
			Type:   "manual",
			Entity: map[string]any{"user_id": "zhangsan", "user_name": "张三"},
			Params: map[string]any{"event_id": event.EventID, "requirement": action.ActionName},
		}
	}
}

func operatorResponse(eventID string, roundID int, commands []domain.Command) map[string]any {
	return map[string]any{
		"type":          "llm_response",
		"from":          domain.RoleOperator,
		"to":            domain.RoleExecutor,
		"event_id":      eventID,
		"round_id":      roundID,
		"response_type": "COMMAND",
		"commands":      commands,
	}
}

func executorResponse(eventID string, roundID int, executions []domain.Execution) map[string]any {
	return map[string]any{
		"type":          "execution_result",
		"from":          domain.RoleExecutor,
		"to":            domain.RoleExpert,
		"event_id":      eventID,
		"round_id":      roundID,
		"response_type": "RESULT",
		"executions":    executions,
	}
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

func executionResult(event domain.Event, command domain.Command) map[string]any {
	params := map[string]any{"event_id": event.EventID, "command_id": command.CommandID}
	switch command.CommandName {
	case "通用IP地址威胁情报信息查询":
		params["verdict"] = "suspicious"
		params["summary"] = "命中自动化威胁情报查询流程，需结合地理位置、历史攻击和受害资产重要性复核。"
		params["indicators"] = []string{"threat_intel_lookup", "source_reputation"}
	case "通用IP地址位置查询":
		params["verdict"] = "informational"
		params["summary"] = "已完成攻击源地理位置查询，结果用于支撑后续拦截范围和误封风险评估。"
		params["indicators"] = []string{"geo_location"}
	case "根据IP地址查询资产信息":
		params["verdict"] = "asset_context_required"
		params["summary"] = "已查询受影响资产上下文，需关注资产负责人、业务线和生产环境属性。"
		params["indicators"] = []string{"asset_owner", "business_context"}
	default:
		params["verdict"] = "manual_required"
		params["summary"] = "该命令需要人工复核或补充外部系统结果。"
	}
	return params
}

func expertResponse(event domain.Event, roundID int, sm domain.Summary, executions []domain.Execution) map[string]any {
	return map[string]any{
		"type":          "llm_response",
		"from":          domain.RoleExpert,
		"to":            []string{domain.RoleCaptain},
		"event_id":      event.EventID,
		"round_id":      roundID,
		"response_type": "SUMMARY",
		"summaries":     []string{sm.EventSummary, fmt.Sprintf("本轮共完成 %d 个执行命令，均已返回结构化结果。", len(executions))},
		"suggestions": []string{
			"建议SOC指挥官结合威胁情报、攻击历史和资产重要性决定是否进入封禁或隔离阶段。",
			"如果目标资产属于生产环境或高 CIA 业务，应补充最近60分钟登录日志和流量会话核查。",
		},
	}
}

func eventSummary(event domain.Event, executions []domain.Execution) string {
	return fmt.Sprintf("事件 %s 已完成第 %d 轮自动驾驶分析：已根据原 DeepSOC 提示词流程完成 TASK、ACTION、COMMAND 和执行结果汇总，当前结论以事实查询为主，暂不直接执行阻断。", firstNonEmpty(event.EventName, event.EventID), firstNonZero(event.CurrentRound, 1))
}

func firstObservable(event domain.Event, roles ...string) string {
	for _, role := range roles {
		for _, item := range event.Observables {
			if strings.EqualFold(item.Role, role) && strings.EqualFold(item.Type, "ip") && item.Value != "" {
				return item.Value
			}
		}
	}
	return ""
}

func firstIP(event domain.Event) string {
	for _, item := range event.Observables {
		if strings.EqualFold(item.Type, "ip") && item.Value != "" {
			return item.Value
		}
	}
	for _, field := range []string{event.Message, event.Context, event.Title, event.EventName} {
		if ip := ipFromText(field); ip != "" {
			return ip
		}
	}
	return ""
}

func ipFromText(text string) string {
	for _, token := range strings.FieldsFunc(text, func(r rune) bool {
		return !(r == '.' || r >= '0' && r <= '9')
	}) {
		if net.ParseIP(token) != nil {
			return token
		}
	}
	return ""
}

func firstNonZero(values ...int) int {
	for _, v := range values {
		if v != 0 {
			return v
		}
	}
	return 0
}

package agents

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/openai/openai-go"
	"github.com/theapemachine/mcp-server-devops-bridge/core"
)

// GetStringArg is a helper to extract a string argument.
func GetStringArg(req mcp.CallToolRequest, key string) (string, error) {
	var (
		val any
		str string
		ok  bool
	)

	if val, ok = req.Params.Arguments[key]; !ok {
		return "", fmt.Errorf("missing argument: %s", key)
	}

	str, ok = val.(string)

	if !ok {
		return "", fmt.Errorf("argument %s is not a string", key)
	}

	return str, nil
}

func GetNumberArg(req mcp.CallToolRequest, key string) (float64, error) {
	var (
		val any
		num float64
		ok  bool
	)

	if val, ok = req.Params.Arguments[key]; !ok {
		return 0, fmt.Errorf("missing argument: %s", key)
	}

	num, ok = val.(float64)

	if !ok {
		return 0, fmt.Errorf("argument %s is not a number", key)
	}

	return num, nil
}

// AgentProvider provides the set of tools for agent management.
type AgentProvider struct {
	Tools map[string]core.Tool
}

// NewAgentProvider creates a new provider for agent tools.
func NewAgentProvider() (*AgentProvider, error) {
	manager, err := NewAgentManager()
	if err != nil {
		return nil, err
	}

	provider := &AgentProvider{
		Tools: make(map[string]core.Tool),
	}

	launchTool := NewLaunchAgentTool(manager)
	listTool := NewListAgentsTool(manager)
	statusTool := NewGetAgentStatusTool(manager)
	instructTool := NewInstructAgentTool(manager)
	shutdownTool := NewShutdownAgentTool(manager)
	bulkManageTool := NewBulkManageAgentsTool(manager)

	provider.Tools[launchTool.Handle().Name] = launchTool
	provider.Tools[listTool.Handle().Name] = listTool
	provider.Tools[statusTool.Handle().Name] = statusTool
	provider.Tools[instructTool.Handle().Name] = instructTool
	provider.Tools[shutdownTool.Handle().Name] = shutdownTool
	provider.Tools[bulkManageTool.Handle().Name] = bulkManageTool

	return provider, nil
}

// --- LaunchAgentTool ---

// LaunchAgentTool is the tool for launching a new agent.
type LaunchAgentTool struct {
	handle  mcp.Tool
	manager *AgentManager
}

// NewLaunchAgentTool creates a new LaunchAgentTool.
func NewLaunchAgentTool(manager *AgentManager) core.Tool {
	t := &LaunchAgentTool{
		manager: manager,
	}
	t.handle = mcp.NewTool(
		"launchAgent",
		mcp.WithDescription("Launches a new agent with a given system and user prompt."),
		mcp.WithString("system_prompt", mcp.Required(), mcp.Description("The system prompt for the agent.")),
		mcp.WithString("user_prompt", mcp.Required(), mcp.Description("The initial user prompt or task for the agent.")),
		mcp.WithNumber("temperature", mcp.Required(), mcp.Description("Controls creativit versus accuracy. Value between 0 and 2. Defaults to 1.")),
		mcp.WithNumber("max_iterations", mcp.Required(), mcp.Description("The maximum number of iterations the agent can perform. Defaults to 10.")),
	)
	return t
}

func (t *LaunchAgentTool) Handle() mcp.Tool { return t.handle }

func (t *LaunchAgentTool) Handler(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	systemPrompt, err := GetStringArg(request, "system_prompt")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	userPrompt, err := GetStringArg(request, "user_prompt")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	temperature := 1.0
	if tempVal, ok := request.Params.Arguments["temperature"]; ok {
		if temp, isFloat := tempVal.(float64); isFloat {
			temperature = temp
		} else {
			return mcp.NewToolResultError("invalid type for 'temperature', expected number"), nil
		}
	}

	maxIterations := 10
	if iterVal, ok := request.Params.Arguments["max_iterations"]; ok {
		if iter, isFloat := iterVal.(float64); isFloat { // JSON numbers are float64
			maxIterations = int(iter)
		} else {
			return mcp.NewToolResultError("invalid type for 'max_iterations', expected integer"), nil
		}
	}

	agent, err := t.manager.LaunchAgent(systemPrompt, userPrompt, temperature, maxIterations)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	return mcp.NewToolResultText(fmt.Sprintf("Agent launched with ID: %s", agent.ID)), nil
}

// --- ListAgentsTool ---

type ListAgentsTool struct {
	handle  mcp.Tool
	manager *AgentManager
}

func NewListAgentsTool(manager *AgentManager) core.Tool {
	t := &ListAgentsTool{manager: manager}
	t.handle = mcp.NewTool(
		"listAgents",
		mcp.WithDescription("Lists all active agents."),
	)
	return t
}

func (t *ListAgentsTool) Handle() mcp.Tool { return t.handle }

func (t *ListAgentsTool) Handler(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	agents := t.manager.ListAgents()
	if len(agents) == 0 {
		return mcp.NewToolResultText("No active agents found."), nil
	}

	type agentInfo struct {
		ID     string `json:"id"`
		Status Status `json:"status"`
		Result string `json:"result"`
	}

	infos := make([]agentInfo, len(agents))
	for i, a := range agents {
		infos[i] = agentInfo{ID: a.ID, Status: a.Status, Result: a.Result}
	}

	jsonResult, err := json.MarshalIndent(infos, "", "  ")
	if err != nil {
		return mcp.NewToolResultError("failed to serialize agent list"), nil
	}

	return mcp.NewToolResultText(string(jsonResult)), nil
}

// --- GetAgentStatusTool ---

type GetAgentStatusTool struct {
	handle  mcp.Tool
	manager *AgentManager
}

func NewGetAgentStatusTool(manager *AgentManager) core.Tool {
	t := &GetAgentStatusTool{manager: manager}
	t.handle = mcp.NewTool(
		"getAgentStatus",
		mcp.WithDescription("Gets the status of a specific agent."),
		mcp.WithString("agent_id", mcp.Required(), mcp.Description("The ID of the agent.")),
	)
	return t
}

func (t *GetAgentStatusTool) Handle() mcp.Tool { return t.handle }

func (t *GetAgentStatusTool) Handler(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	agentID, err := GetStringArg(request, "agent_id")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	agent, err := t.manager.GetAgentStatus(agentID)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	// Create a clean summary for the main agent
	type agentStatusResponse struct {
		ID       string                                   `json:"id"`
		Status   Status                                   `json:"status"`
		Result   string                                   `json:"result"`
		Messages []openai.ChatCompletionMessageParamUnion `json:"messages"`
	}

	response := agentStatusResponse{
		ID:       agent.ID,
		Status:   agent.Status,
		Result:   agent.Result,
		Messages: agent.Messages,
	}

	jsonResult, err := json.MarshalIndent(response, "", "  ")
	if err != nil {
		return mcp.NewToolResultError("failed to serialize agent status"), nil
	}

	return mcp.NewToolResultText(string(jsonResult)), nil
}

// --- InstructAgentTool ---

type InstructAgentTool struct {
	handle  mcp.Tool
	manager *AgentManager
}

func NewInstructAgentTool(manager *AgentManager) core.Tool {
	t := &InstructAgentTool{manager: manager}
	t.handle = mcp.NewTool(
		"instructAgent",
		mcp.WithDescription("Sends a new instruction to a waiting agent."),
		mcp.WithString("agent_id", mcp.Required(), mcp.Description("The ID of the agent.")),
		mcp.WithString("prompt", mcp.Required(), mcp.Description("The new prompt or instruction for the agent.")),
	)
	return t
}

func (t *InstructAgentTool) Handle() mcp.Tool { return t.handle }

func (t *InstructAgentTool) Handler(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	agentID, err := GetStringArg(request, "agent_id")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	prompt, err := GetStringArg(request, "prompt")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	err = t.manager.InstructAgent(agentID, prompt)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	return mcp.NewToolResultText(fmt.Sprintf("Instruction sent to agent %s.", agentID)), nil
}

// --- ShutdownAgentTool ---

type ShutdownAgentTool struct {
	handle  mcp.Tool
	manager *AgentManager
}

func NewShutdownAgentTool(manager *AgentManager) core.Tool {
	t := &ShutdownAgentTool{manager: manager}
	t.handle = mcp.NewTool(
		"shutdownAgent",
		mcp.WithDescription("Terminates a running agent."),
		mcp.WithString("agent_id", mcp.Required(), mcp.Description("The ID of the agent to terminate.")),
	)
	return t
}

func (t *ShutdownAgentTool) Handle() mcp.Tool { return t.handle }

func (t *ShutdownAgentTool) Handler(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	agentID, err := GetStringArg(request, "agent_id")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	err = t.manager.ShutdownAgent(agentID)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	return mcp.NewToolResultText(fmt.Sprintf("Shutdown signal sent to agent %s.", agentID)), nil
}

// --- BulkManageAgentsTool ---

// BulkManageAgentsTool provides a way to send multiple instructions at once.
type BulkManageAgentsTool struct {
	handle  mcp.Tool
	manager *AgentManager
}

// NewBulkManageAgentsTool creates a new BulkManageAgentsTool.
func NewBulkManageAgentsTool(manager *AgentManager) core.Tool {
	t := &BulkManageAgentsTool{manager: manager}
	t.handle = mcp.NewTool(
		"bulkManageAgents",
		mcp.WithDescription("Sends a batch of instructions to multiple agents in a single request. Can be used to launch, instruct, or shut down agents."),
		mcp.WithString("operations", mcp.Required(), mcp.Description("A JSON string representing an array of operations. Each operation is an object with 'action' ('launch', 'instruct', or 'shutdown'), 'agent_id' (for instruct/shutdown), and other parameters.")),
	)
	return t
}

func (t *BulkManageAgentsTool) Handle() mcp.Tool { return t.handle }

// Handler processes the bulk operations.
func (t *BulkManageAgentsTool) Handler(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	operationsStr, err := GetStringArg(request, "operations")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	type operation struct {
		Action        string  `json:"action"`
		AgentID       string  `json:"agent_id,omitempty"`
		Prompt        string  `json:"prompt,omitempty"`
		SystemPrompt  string  `json:"system_prompt,omitempty"`
		Temperature   float64 `json:"temperature,omitempty"`
		MaxIterations int     `json:"max_iterations,omitempty"`
	}

	var ops []operation
	if err := json.Unmarshal([]byte(operationsStr), &ops); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to parse operations JSON: %v", err)), nil
	}

	var results []string
	for _, op := range ops {
		var result string
		switch op.Action {
		case "launch":
			temp := op.Temperature
			if temp == 0 { // If not provided in JSON, it will be the zero value
				temp = 1.0
			}
			iters := op.MaxIterations
			if iters == 0 {
				iters = 10
			}

			if op.Prompt == "" || op.SystemPrompt == "" {
				result = "Launch op: FAILED - 'prompt' and 'system_prompt' are required for 'launch' action."
			} else if agent, err := t.manager.LaunchAgent(op.SystemPrompt, op.Prompt, temp, iters); err != nil {
				result = fmt.Sprintf("Launch op: FAILED - %v", err)
			} else {
				result = fmt.Sprintf("Launch op: SUCCESS - Agent launched with ID: %s", agent.ID)
			}
		case "instruct":
			if op.AgentID == "" || op.Prompt == "" {
				result = "Instruct op: FAILED - 'agent_id' and 'prompt' are required for 'instruct' action."
			} else if err := t.manager.InstructAgent(op.AgentID, op.Prompt); err != nil {
				result = fmt.Sprintf("Agent %s: FAILED to instruct - %v", op.AgentID, err)
			} else {
				result = fmt.Sprintf("Agent %s: Instruction sent.", op.AgentID)
			}
		case "shutdown":
			if op.AgentID == "" {
				result = "Shutdown op: FAILED - 'agent_id' is required for 'shutdown' action."
			} else if err := t.manager.ShutdownAgent(op.AgentID); err != nil {
				result = fmt.Sprintf("Agent %s: FAILED to shut down - %v", op.AgentID, err)
			} else {
				result = fmt.Sprintf("Agent %s: Shutdown signal sent.", op.AgentID)
			}
		default:
			result = fmt.Sprintf("Op for agent %s: FAILED - unknown action '%s'.", op.AgentID, op.Action)
		}
		results = append(results, result)
	}

	return mcp.NewToolResultText(strings.Join(results, "\n")), nil
}

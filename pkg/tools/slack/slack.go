package slack

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/slack-go/slack"
	"github.com/theapemachine/mcp-server-devops-bridge/core"
)

// SlackPostMessageTool is a tool for posting messages to a Slack channel.
type SlackPostMessageTool struct {
	client           *slack.Client // Changed from WebhookURL
	defaultChannelID string        // Added for default channel
	handle           mcp.Tool
}

// NewSlackPostMessageTool creates a new SlackPostMessageTool.
func NewSlackPostMessageTool() core.Tool {
	botToken := os.Getenv("SLACK_BOT_TOKEN")
	defaultChannelID := os.Getenv("SLACK_DEFAULT_CHANNEL_ID")

	if botToken == "" {
		return nil
	}

	api := slack.New(botToken)

	t := &SlackPostMessageTool{
		client:           api,
		defaultChannelID: defaultChannelID,
	}

	t.handle = mcp.NewTool(
		"post_slack_message",
		mcp.WithDescription("Posts a message to a Slack channel. Uses SLACK_DEFAULT_CHANNEL_ID if channel_id is not provided."),
		mcp.WithString(
			"message",
			mcp.Required(),
			mcp.Description("The message text to post to Slack."),
		),
		mcp.WithString(
			"channel_id",
			mcp.Description("Optional. The ID of the channel to post to. If not provided, uses the default configured channel."),
		),
	)
	return t
}

// Handle returns the tool's definition.
func (t *SlackPostMessageTool) Handle() mcp.Tool {
	return t.handle
}

// Handler executes the tool.
func (t *SlackPostMessageTool) Handler(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	message, ok := request.Params.Arguments["message"].(string)
	if !ok || message == "" {
		return mcp.NewToolResultError("invalid_argument: message argument is missing, empty, or not a string"), nil
	}

	channelID := t.defaultChannelID
	if reqChannelID, reqChannelOk := request.Params.Arguments["channel_id"].(string); reqChannelOk && reqChannelID != "" {
		channelID = reqChannelID
	}

	if channelID == "" {
		return mcp.NewToolResultError("missing_configuration: channel_id is not provided and SLACK_DEFAULT_CHANNEL_ID is not set."), nil
	}

	postedChannelID, timestamp, err := t.client.PostMessageContext(ctx,
		channelID,
		slack.MsgOptionText(message, false),
	)

	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("slack_api_error: failed to send message to Slack channel %s: %v", channelID, err)), nil
	}

	responseData := map[string]interface{}{
		"status":       "success",
		"channel_id":   postedChannelID,
		"timestamp":    timestamp,
		"message_sent": message,
	}
	jsonResponse, err := json.Marshal(responseData)

	if err != nil {
		return mcp.NewToolResultError("internal_error: failed to create JSON response"), nil
	}

	return mcp.NewToolResultText(string(jsonResponse)), nil
}

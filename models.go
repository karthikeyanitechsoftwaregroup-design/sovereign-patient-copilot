package models

import "time"

// PatientRecord is the de-identified record stored in Postgres.
type PatientRecord struct {
	ID            int64     `json:"id"`
	Filename      string    `json:"filename"`
	RawText       string    `json:"raw_text,omitempty"`
	ScribeSummary string    `json:"scribe_summary"`
	CreatedAt     time.Time `json:"created_at"`
}

// Suggestion is a next-best-action from Agent B.
type Suggestion struct {
	ID          int64     `json:"id"`
	RecordID    int64     `json:"record_id"`
	Title       string    `json:"title"`
	Description string    `json:"description"`
	Priority    string    `json:"priority"` // urgent | high | moderate
	Citation    string    `json:"citation"`
	CreatedAt   time.Time `json:"created_at"`
}

// SSEEvent is streamed to the frontend.
type SSEEvent struct {
	Type    string `json:"type"`    // pipeline_stage | scribe_chunk | suggestions | error
	Stage   int    `json:"stage,omitempty"`
	Payload any    `json:"payload,omitempty"`
}

// AnthropicRequest mirrors the /v1/messages body we send.
type AnthropicRequest struct {
	Model     string             `json:"model"`
	MaxTokens int                `json:"max_tokens"`
	System    string             `json:"system,omitempty"`
	Messages  []AnthropicMessage `json:"messages"`
	Stream    bool               `json:"stream,omitempty"`
}

type AnthropicMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// AnthropicResponse (non-streaming).
type AnthropicResponse struct {
	Content []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content"`
}

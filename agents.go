package agents

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"sovereign/internal/models"
)

const anthropicURL = "https://api.anthropic.com/v1/messages"
const anthropicVersion = "2023-06-01"

// ─── AI routing ─────────────────────────────────────────────────────────────
// Agent A (Scribe) → needs precise instruction following → claude-haiku (fast, cheap)
// Agent B (Specialist) → needs deep clinical reasoning → claude-sonnet (powerful)

func modelForRole(role string) string {
	switch role {
	case "scribe":
		// Fast, cost-efficient for extraction / de-identification
		return "claude-haiku-4-5-20251001"
	case "specialist":
		// More capable for nuanced clinical reasoning
		return "claude-sonnet-4-6"
	default:
		return "claude-sonnet-4-6"
	}
}

// ─── Agent A: Scribe ─────────────────────────────────────────────────────────

const scribeSystem = `You are a clinical scribe AI (Agent A).
Your responsibilities:
1. Remove all Protected Health Information (PHI): patient name, DOB, MRN, address, phone, email, SSN.
   Replace each with [REDACTED].
2. Extract structured data: vitals, diagnoses, medications, recent labs, ECG/imaging findings.
3. Write a concise clinical summary (4–6 sentences) for the treating physician.
Respond ONLY with the clinical summary paragraph. No headers, no lists, no PHI.`

// RunScribe calls Agent A and returns a de-identified clinical summary.
func RunScribe(ctx context.Context, pdfText string) (string, error) {
	req := models.AnthropicRequest{
		Model:     modelForRole("scribe"),
		MaxTokens: 1024,
		System:    scribeSystem,
		Messages: []models.AnthropicMessage{
			{Role: "user", Content: "Patient PDF text:\n\n" + pdfText},
		},
	}
	return callAnthropic(ctx, req)
}

// ─── Agent B: Specialist ─────────────────────────────────────────────────────

const specialistSystem = `You are a clinical specialist AI (Agent B).
You receive a de-identified patient summary from Agent A (Scribe).
Generate exactly 4 evidence-based "Next Best Actions" for the treating physician.
Respond ONLY as a valid JSON array — no markdown fences, no preamble:
[
  {
    "title": "Short action title",
    "description": "One to two sentence clinical rationale",
    "priority": "urgent|high|moderate",
    "citation": "Exact finding from summary that triggered this action"
  }
]`

// Suggestion mirrors the JSON Agent B returns (before DB persist).
type Suggestion struct {
	Title       string `json:"title"`
	Description string `json:"description"`
	Priority    string `json:"priority"`
	Citation    string `json:"citation"`
}

// RunSpecialist calls Agent B and parses the JSON suggestion list.
func RunSpecialist(ctx context.Context, scribeSummary string) ([]Suggestion, error) {
	req := models.AnthropicRequest{
		Model:     modelForRole("specialist"),
		MaxTokens: 1024,
		System:    specialistSystem,
		Messages: []models.AnthropicMessage{
			{Role: "user", Content: "De-identified patient summary:\n\n" + scribeSummary},
		},
	}

	raw, err := callAnthropic(ctx, req)
	if err != nil {
		return nil, err
	}

	// Strip accidental markdown fences
	raw = strings.TrimSpace(raw)
	raw = strings.TrimPrefix(raw, "```json")
	raw = strings.TrimPrefix(raw, "```")
	raw = strings.TrimSuffix(raw, "```")
	raw = strings.TrimSpace(raw)

	var suggestions []Suggestion
	if err := json.Unmarshal([]byte(raw), &suggestions); err != nil {
		return nil, fmt.Errorf("parse specialist JSON: %w\nraw: %s", err, raw)
	}
	return suggestions, nil
}

// ─── HTTP helper ─────────────────────────────────────────────────────────────

func callAnthropic(ctx context.Context, req models.AnthropicRequest) (string, error) {
	apiKey := os.Getenv("ANTHROPIC_API_KEY")
	if apiKey == "" {
		return "", fmt.Errorf("ANTHROPIC_API_KEY not set")
	}

	body, _ := json.Marshal(req)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, anthropicURL, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("x-api-key", apiKey)
	httpReq.Header.Set("anthropic-version", anthropicVersion)

	resp, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		return "", fmt.Errorf("anthropic http: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("anthropic %d: %s", resp.StatusCode, respBody)
	}

	var ar models.AnthropicResponse
	if err := json.Unmarshal(respBody, &ar); err != nil {
		return "", fmt.Errorf("decode anthropic response: %w", err)
	}
	if len(ar.Content) == 0 {
		return "", fmt.Errorf("empty content from anthropic")
	}
	return ar.Content[0].Text, nil
}

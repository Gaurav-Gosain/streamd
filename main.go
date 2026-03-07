package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	styles "charm.land/glamour/v2/styles"

	"charm.land/fang/v2"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

// Version information (set by goreleaser).
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

type options struct {
	alt     bool
	noThink bool
	info    bool
	style   string
	wrap    int
}

// sseEvent is the internal representation of a parsed streaming event.
type sseEvent struct {
	ReasoningContent string
	Content          string
	Done             bool
	Meta             *streamMeta
}

// streamMeta carries optional metadata from the final chunk(s).
type streamMeta struct {
	Model            string
	FinishReason     string
	PromptTokens     int
	CompletionTokens int
	TotalTokens      int
	// Ollama-specific
	TotalDuration   time.Duration
	PromptEvalCount int
	EvalCount       int
}

// universalChunk handles OpenAI Chat Completions, Ollama /api/chat,
// Ollama /api/generate, and non-streaming responses.
type universalChunk struct {
	Choices []struct {
		Delta struct {
			Content          string `json:"content"`
			ReasoningContent string `json:"reasoning_content"`
		} `json:"delta"`
		Message struct {
			Content          string `json:"content"`
			ReasoningContent string `json:"reasoning_content"`
		} `json:"message"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`

	Model string `json:"model"`
	Usage *struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
	} `json:"usage"`

	// Ollama /api/chat
	Message struct {
		Content string `json:"content"`
	} `json:"message"`

	// Ollama /api/generate
	Response string `json:"response"`

	// Ollama flags & metadata
	Done            bool   `json:"done"`
	DoneReason      string `json:"done_reason"`
	TotalDuration   int64  `json:"total_duration"`
	PromptEvalCount int    `json:"prompt_eval_count"`
	EvalCount       int    `json:"eval_count"`
}

// responsesEvent handles OpenAI Responses API streaming events.
type responsesEvent struct {
	Type     string `json:"type"`
	Delta    string `json:"delta"`
	Response *struct {
		Model string `json:"model"`
		Usage *struct {
			InputTokens  int `json:"input_tokens"`
			OutputTokens int `json:"output_tokens"`
			TotalTokens  int `json:"total_tokens"`
		} `json:"usage"`
	} `json:"response"`
}

// parseLine auto-detects the format and extracts content from a line of input.
func parseLine(line string) sseEvent {
	// SSE: lines prefixed with "data: "
	if data, ok := strings.CutPrefix(line, "data: "); ok {
		if data == "[DONE]" {
			return sseEvent{Done: true}
		}
		return parseJSON(data)
	}

	// Skip empty lines and SSE comments/event lines
	trimmed := strings.TrimSpace(line)
	if trimmed == "" || strings.HasPrefix(trimmed, "event:") || strings.HasPrefix(trimmed, ":") {
		return sseEvent{}
	}

	// JSON line: try structured formats first
	if trimmed[0] == '{' {
		if ev := parseJSON(trimmed); ev.Content != "" || ev.ReasoningContent != "" || ev.Done || ev.Meta != nil {
			return ev
		}
	}

	// Plain text fallback (e.g. ollama run, cat, echo)
	return sseEvent{Content: line + "\n"}
}

func parseJSON(data string) sseEvent {
	// Check for Responses API events (have a "type" field)
	var peek struct {
		Type string `json:"type"`
	}
	if json.Unmarshal([]byte(data), &peek) == nil && peek.Type != "" {
		return parseResponsesEvent(data, peek.Type)
	}

	var c universalChunk
	if err := json.Unmarshal([]byte(data), &c); err != nil {
		return sseEvent{}
	}

	// OpenAI Chat Completions format
	if len(c.Choices) > 0 {
		ch := c.Choices[0]

		// Streaming usage-only chunk (empty choices with usage)
		if ch.Delta.Content == "" && ch.Delta.ReasoningContent == "" &&
			ch.Message.Content == "" && ch.Message.ReasoningContent == "" &&
			c.Usage != nil {
			return sseEvent{
				Meta: buildMetaFromChat(&c, ch.FinishReason),
			}
		}

		// Streaming: delta field
		if ch.Delta.Content != "" || ch.Delta.ReasoningContent != "" {
			ev := sseEvent{
				Content:          ch.Delta.Content,
				ReasoningContent: ch.Delta.ReasoningContent,
			}
			if ch.FinishReason != "" {
				ev.Meta = buildMetaFromChat(&c, ch.FinishReason)
			}
			return ev
		}

		// Non-streaming: message field
		if ch.Message.Content != "" || ch.Message.ReasoningContent != "" {
			return sseEvent{
				Content:          ch.Message.Content,
				ReasoningContent: ch.Message.ReasoningContent,
				Done:             true,
				Meta:             buildMetaFromChat(&c, ch.FinishReason),
			}
		}
	}

	// Ollama /api/chat
	if c.Message.Content != "" {
		ev := sseEvent{Content: c.Message.Content, Done: c.Done}
		if c.Done {
			ev.Meta = buildMetaFromOllama(&c)
		}
		return ev
	}

	// Ollama /api/generate
	if c.Response != "" {
		ev := sseEvent{Content: c.Response, Done: c.Done}
		if c.Done {
			ev.Meta = buildMetaFromOllama(&c)
		}
		return ev
	}

	// Ollama final metadata-only chunk (done:true, no content)
	if c.Done {
		return sseEvent{Done: true, Meta: buildMetaFromOllama(&c)}
	}

	return sseEvent{}
}

func parseResponsesEvent(data, typ string) sseEvent {
	var re responsesEvent
	if err := json.Unmarshal([]byte(data), &re); err != nil {
		return sseEvent{}
	}

	switch typ {
	case "response.output_text.delta":
		return sseEvent{Content: re.Delta}
	case "response.reasoning_summary_text.delta":
		return sseEvent{ReasoningContent: re.Delta}
	case "response.completed":
		ev := sseEvent{Done: true}
		if re.Response != nil {
			meta := &streamMeta{Model: re.Response.Model}
			if u := re.Response.Usage; u != nil {
				meta.PromptTokens = u.InputTokens
				meta.CompletionTokens = u.OutputTokens
				meta.TotalTokens = u.TotalTokens
			}
			ev.Meta = meta
		}
		return ev
	case "response.failed", "response.incomplete":
		return sseEvent{Done: true}
	}

	return sseEvent{}
}

func buildMetaFromChat(c *universalChunk, finishReason string) *streamMeta {
	m := &streamMeta{
		Model:        c.Model,
		FinishReason: finishReason,
	}
	if c.Usage != nil {
		m.PromptTokens = c.Usage.PromptTokens
		m.CompletionTokens = c.Usage.CompletionTokens
		m.TotalTokens = c.Usage.TotalTokens
	}
	return m
}

func buildMetaFromOllama(c *universalChunk) *streamMeta {
	return &streamMeta{
		Model:           c.Model,
		FinishReason:    c.DoneReason,
		TotalDuration:   time.Duration(c.TotalDuration),
		PromptEvalCount: c.PromptEvalCount,
		EvalCount:       c.EvalCount,
	}
}

func main() {
	var opts options

	cmd := &cobra.Command{
		Use:   "streamd",
		Short: "Stream and render markdown from LLM endpoints",
		Long: `Streams responses from OpenAI-compatible, Ollama, and Responses API
endpoints and renders the markdown output in the terminal using glamour.

Supported formats:
  - OpenAI Chat Completions SSE (streaming and non-streaming)
  - OpenAI Responses API (response.output_text.delta events)
  - Ollama /api/chat (NDJSON)
  - Ollama /api/generate (NDJSON)

Supports thinking/reasoning tokens via the 'reasoning_content' field
and inline <think>...</think> tags.`,
		Example: `  # OpenAI-compatible endpoint:
  curl -s https://api.example.com/v1/chat/completions \
    -H "Content-Type: application/json" \
    -d '{"model":"...","messages":[...],"stream":true}' | streamd

  # Ollama native:
  curl -s http://localhost:11434/api/chat \
    -d '{"model":"gemma3:4b","messages":[...],"stream":true}' | streamd

  # With usage info:
  curl -s ... | streamd --info

  # Alt-screen with scrollable viewport:
  curl -s ... | streamd --alt

  # Hide thinking:
  curl -s ... | streamd --no-think`,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if term.IsTerminal(int(os.Stdin.Fd())) {
				return cmd.Help()
			}
			return run(opts)
		},
	}

	cmd.Flags().BoolVar(&opts.alt, "alt", false, "interactive alt-screen with viewport scrolling")
	cmd.Flags().BoolVar(&opts.noThink, "no-think", false, "hide thinking/reasoning output")
	cmd.Flags().BoolVarP(&opts.info, "info", "i", false, "show model and token usage after response")
	cmd.Flags().StringVar(&opts.style, "style", "", `glamour style: dark, light, dracula, tokyo-night, pink, ascii`)
	cmd.Flags().IntVarP(&opts.wrap, "wrap", "w", 0, "wrap width (0 = terminal width)")

	if err := fang.Execute(context.Background(), cmd,
		fang.WithVersion(fmt.Sprintf("%s\nCommit: %s\nBuilt:  %s", version, commit, date)),
	); err != nil {
		os.Exit(1)
	}
}

func resolveStyle(s string) string {
	if s != "" {
		return s
	}
	if e := os.Getenv("GLAMOUR_STYLE"); e != "" {
		return e
	}
	return styles.DarkStyle
}

func termWidth(override int) int {
	if override > 0 {
		return override
	}
	if w, _, err := term.GetSize(int(os.Stdout.Fd())); err == nil && w > 0 {
		return w
	}
	return 80
}

func run(opts options) error {
	style := resolveStyle(opts.style)
	width := termWidth(opts.wrap)
	showThink := !opts.noThink

	if opts.alt {
		return runTUI(style, width, showThink, opts.info)
	}
	return runInline(style, width, showThink, opts.info)
}

// nextWordBoundary returns the byte offset just past the next word after pos.
func nextWordBoundary(s string, pos int) int {
	i := strings.IndexAny(s[pos:], " \t\n")
	if i < 0 {
		return len(s)
	}
	return pos + i + 1
}

func printMeta(m *streamMeta) {
	dim := "\033[2m"
	reset := "\033[0m"

	var parts []string
	if m.Model != "" {
		parts = append(parts, "model: "+m.Model)
	}
	if m.FinishReason != "" {
		parts = append(parts, "finish: "+m.FinishReason)
	}
	if m.TotalTokens > 0 {
		parts = append(parts, fmt.Sprintf("tokens: %d prompt + %d completion = %d total",
			m.PromptTokens, m.CompletionTokens, m.TotalTokens))
	} else if m.PromptEvalCount > 0 || m.EvalCount > 0 {
		parts = append(parts, fmt.Sprintf("tokens: %d prompt + %d eval",
			m.PromptEvalCount, m.EvalCount))
	}
	if m.TotalDuration > 0 {
		parts = append(parts, fmt.Sprintf("duration: %s", m.TotalDuration.Round(time.Millisecond)))
		if m.EvalCount > 0 {
			tps := float64(m.EvalCount) / m.TotalDuration.Seconds()
			parts = append(parts, fmt.Sprintf("speed: %.1f tok/s", tps))
		}
	}

	if len(parts) > 0 {
		fmt.Fprintf(os.Stderr, "%s%s%s\n", dim, strings.Join(parts, "  |  "), reset)
	}
}

// parseContent splits text into thinking/content deltas, handling <think> tags.
func parseContent(text string, inThink bool) (thinkDelta, contentDelta string, newInThink bool) {
	var tb, cb strings.Builder
	rest := text
	for len(rest) > 0 {
		if inThink {
			if i := strings.Index(rest, "</think>"); i >= 0 {
				tb.WriteString(rest[:i])
				rest = rest[i+8:]
				inThink = false
			} else {
				tb.WriteString(rest)
				rest = ""
			}
		} else {
			if i := strings.Index(rest, "<think>"); i >= 0 {
				cb.WriteString(rest[:i])
				rest = rest[i+7:]
				inThink = true
			} else {
				cb.WriteString(rest)
				rest = ""
			}
		}
	}
	return tb.String(), cb.String(), inThink
}

// buildMd composes the final markdown from thinking + content.
func buildMd(thinking, content string, showThink bool) string {
	var sb strings.Builder
	if showThink && thinking != "" {
		sb.WriteString("*thinking...*\n\n")
		for line := range strings.SplitSeq(strings.TrimRight(thinking, "\n"), "\n") {
			sb.WriteString("> ")
			sb.WriteString(line)
			sb.WriteString("\n")
		}
		if content != "" {
			sb.WriteString("\n---\n\n")
		}
	}
	sb.WriteString(content)
	return sb.String()
}

// readEvents reads streaming events from stdin (any supported format) and sends them to ch.
func readEvents(ch chan<- sseEvent) {
	defer close(ch)
	scanner := bufio.NewScanner(os.Stdin)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	for scanner.Scan() {
		ev := parseLine(scanner.Text())
		if ev.Content != "" || ev.ReasoningContent != "" || ev.Meta != nil {
			ch <- sseEvent{
				ReasoningContent: ev.ReasoningContent,
				Content:          ev.Content,
				Meta:             ev.Meta,
			}
		}
		if ev.Done {
			ch <- sseEvent{Done: true, Meta: ev.Meta}
			return
		}
	}
	ch <- sseEvent{Done: true}
}

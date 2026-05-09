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

type sseEvent struct {
	ReasoningContent string
	Content          string
	Done             bool
	Meta             *streamMeta
}

type streamMeta struct {
	Model            string
	FinishReason     string
	PromptTokens     int
	CompletionTokens int
	TotalTokens      int
	TotalDuration    time.Duration
	PromptEvalCount  int
	EvalCount        int
}

// universalChunk covers OpenAI Chat Completions, Ollama /api/chat,
// and Ollama /api/generate formats.
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

	Message  struct{ Content string `json:"content"` } `json:"message"`
	Response string                                     `json:"response"`
	Thinking string                                     `json:"thinking"`

	Done            bool   `json:"done"`
	DoneReason      string `json:"done_reason"`
	TotalDuration   int64  `json:"total_duration"`
	PromptEvalCount int    `json:"prompt_eval_count"`
	EvalCount       int    `json:"eval_count"`
}

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

func parseLine(line string) sseEvent {
	if data, ok := strings.CutPrefix(line, "data: "); ok {
		if data == "[DONE]" {
			return sseEvent{Done: true}
		}
		return parseJSON(data)
	}

	trimmed := strings.TrimSpace(line)
	if trimmed == "" || strings.HasPrefix(trimmed, "event:") || strings.HasPrefix(trimmed, ":") {
		return sseEvent{}
	}

	if trimmed[0] == '{' {
		if ev := parseJSON(trimmed); ev.Content != "" || ev.ReasoningContent != "" || ev.Done || ev.Meta != nil {
			return ev
		}
	}

	return sseEvent{Content: line + "\n"}
}

func parseJSON(data string) sseEvent {
	var peek struct{ Type string `json:"type"` }
	if json.Unmarshal([]byte(data), &peek) == nil && peek.Type != "" {
		return parseResponsesEvent(data, peek.Type)
	}

	var c universalChunk
	if err := json.Unmarshal([]byte(data), &c); err != nil {
		return sseEvent{}
	}

	if len(c.Choices) > 0 {
		ch := c.Choices[0]

		if ch.Delta.Content == "" && ch.Delta.ReasoningContent == "" &&
			ch.Message.Content == "" && ch.Message.ReasoningContent == "" &&
			c.Usage != nil {
			return sseEvent{Meta: buildMetaFromChat(&c, ch.FinishReason)}
		}

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

		if ch.Message.Content != "" || ch.Message.ReasoningContent != "" {
			return sseEvent{
				Content:          ch.Message.Content,
				ReasoningContent: ch.Message.ReasoningContent,
				Done:             true,
				Meta:             buildMetaFromChat(&c, ch.FinishReason),
			}
		}
	}

	if c.Message.Content != "" || c.Thinking != "" {
		ev := sseEvent{Content: c.Message.Content, ReasoningContent: c.Thinking, Done: c.Done}
		if c.Done {
			ev.Meta = buildMetaFromOllama(&c)
		}
		return ev
	}

	if c.Response != "" || c.Thinking != "" {
		ev := sseEvent{Content: c.Response, ReasoningContent: c.Thinking, Done: c.Done}
		if c.Done {
			ev.Meta = buildMetaFromOllama(&c)
		}
		return ev
	}

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
  - Ollama /api/chat and /api/generate (NDJSON)

Supports thinking/reasoning tokens via the 'reasoning_content' field
and inline <think>...</think> tags.`,
		Example: `  # OpenAI-compatible (use -N to disable curl buffering):
  curl -sN https://api.example.com/v1/chat/completions \
    -H "Content-Type: application/json" \
    -d '{"model":"...","messages":[...],"stream":true}' | streamd

  # Ollama:
  curl -sN http://localhost:11434/api/chat \
    -d '{"model":"gemma3:4b","messages":[...],"stream":true}' | streamd

  # With usage info:
  curl -sN ... | streamd --info

  # Interactive alt-screen with scrollable viewport:
  curl -sN ... | streamd --alt

  # Hide thinking:
  curl -sN ... | streamd --no-think`,
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

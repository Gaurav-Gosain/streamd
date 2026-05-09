package main

import (
	"fmt"
	"math"
	"os"
	"strings"

	glamour "charm.land/glamour/v2"

	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	lipgloss "charm.land/lipgloss/v2"
)

const barHeight = 2

var (
	barFilledStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#7aa2f7"))
	barEmptyStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("#292e42"))
	tuiStatusStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#7aa2f7")).Bold(true)
	tuiDimStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("#565f89"))
)

type sseBatchMsg []sseEvent

// waitForSSE blocks until an event arrives, then drains all buffered events.
func waitForSSE(ch <-chan sseEvent) tea.Cmd {
	return func() tea.Msg {
		ev, ok := <-ch
		if !ok {
			return sseBatchMsg{{Done: true}}
		}
		if ev.Done {
			return sseBatchMsg{ev}
		}
		batch := sseBatchMsg{ev}
		for {
			select {
			case ev, ok := <-ch:
				if !ok {
					batch = append(batch, sseEvent{Done: true})
					return batch
				}
				batch = append(batch, ev)
				if ev.Done {
					return batch
				}
			default:
				return batch
			}
		}
	}
}

// applyBatch processes a batch of events into the model's state.
func applyBatch(batch sseBatchMsg, thinking, content *string, inThink *bool, meta **streamMeta) (done bool) {
	for _, ev := range batch {
		if ev.Meta != nil {
			*meta = ev.Meta
		}
		if ev.Done {
			done = true
			continue
		}
		if ev.ReasoningContent != "" {
			*thinking += ev.ReasoningContent
		}
		if ev.Content != "" {
			td, cd, next := parseContent(ev.Content, *inThink)
			*thinking += td
			*content += cd
			*inThink = next
		}
	}
	return done
}

func renderMd(r *glamour.TermRenderer, thinking, content string, showThink bool) string {
	md := buildMd(thinking, content, showThink)
	if md == "" {
		return ""
	}
	out, err := r.Render(md)
	if err != nil {
		return md
	}
	return out
}

// --- Alt-screen (interactive) mode ---

type tuiModel struct {
	viewport   viewport.Model
	width      int
	height     int
	thinking   string
	content    string
	inThink    bool
	showThink  bool
	showInfo   bool
	styleName  string
	events     <-chan sseEvent
	done       bool
	renderer   *glamour.TermRenderer
	autoScroll bool
	meta       *streamMeta
}

func (m tuiModel) Init() tea.Cmd {
	return waitForSSE(m.events)
}

func (m tuiModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		switch msg.String() {
		case "q", "esc", "ctrl+c":
			return m, tea.Quit
		case "G", "end":
			m.viewport.GotoBottom()
			m.autoScroll = true
			return m, nil
		case "g", "home":
			m.viewport.GotoTop()
			m.autoScroll = false
			return m, nil
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.viewport.SetWidth(msg.Width)
		m.viewport.SetHeight(msg.Height - barHeight)
		if r, err := glamour.NewTermRenderer(
			glamour.WithStandardStyle(m.styleName),
			glamour.WithWordWrap(msg.Width),
		); err == nil {
			m.renderer = r
		}
		if m.content != "" || m.thinking != "" {
			m.renderAndSync()
		}

	case sseBatchMsg:
		m.done = applyBatch(msg, &m.thinking, &m.content, &m.inThink, &m.meta)
		m.renderAndSync()
		if m.done {
			return m, nil
		}
		return m, waitForSSE(m.events)
	}

	var cmd tea.Cmd
	m.viewport, cmd = m.viewport.Update(msg)
	if m.viewport.ScrollPercent() < 1.0 {
		m.autoScroll = false
	}
	return m, cmd
}

func (m *tuiModel) renderAndSync() {
	out := renderMd(m.renderer, m.thinking, m.content, m.showThink)
	if out == "" {
		return
	}
	m.viewport.SetContent(out)
	if m.autoScroll {
		m.viewport.GotoBottom()
	}
}

func (m tuiModel) View() tea.View {
	var b strings.Builder
	b.WriteString(m.viewport.View())
	b.WriteString("\n")

	pct := m.viewport.ScrollPercent()
	filled := min(int(math.Round(pct*float64(m.width))), m.width)
	empty := max(0, m.width-filled)
	b.WriteString(barFilledStyle.Render(strings.Repeat("━", filled)))
	b.WriteString(barEmptyStyle.Render(strings.Repeat("─", empty)))
	b.WriteString("\n")

	status := tuiStatusStyle.Render("streaming...")
	if m.done {
		status = tuiStatusStyle.Render("done")
	}

	var info string
	if m.showInfo && m.meta != nil {
		info = tuiDimStyle.Render(formatMeta(m.meta))
	}

	keys := tuiDimStyle.Render("q/esc quit  j/k scroll  g/G top/bottom")

	if info != "" {
		left := status + "  " + info
		gap := max(0, m.width-lipgloss.Width(left)-lipgloss.Width(keys))
		b.WriteString(left)
		b.WriteString(strings.Repeat(" ", gap))
		b.WriteString(keys)
	} else {
		gap := max(0, m.width-lipgloss.Width(status)-lipgloss.Width(keys))
		b.WriteString(status)
		b.WriteString(strings.Repeat(" ", gap))
		b.WriteString(keys)
	}

	v := tea.NewView(b.String())
	v.AltScreen = true
	return v
}

func formatMeta(m *streamMeta) string {
	var parts []string
	if m.Model != "" {
		parts = append(parts, m.Model)
	}
	if m.TotalTokens > 0 {
		parts = append(parts, fmt.Sprintf("%dt", m.TotalTokens))
	} else if m.EvalCount > 0 {
		parts = append(parts, fmt.Sprintf("%dt", m.PromptEvalCount+m.EvalCount))
	}
	if m.TotalDuration > 0 && m.EvalCount > 0 {
		tps := float64(m.EvalCount) / m.TotalDuration.Seconds()
		parts = append(parts, fmt.Sprintf("%.0f tok/s", tps))
	}
	return strings.Join(parts, "  ")
}

// --- Inline (default) mode ---
//
// Streams in alt-screen to keep the main scrollback clean.
// On exit the alt-screen is discarded and the final output is printed.

type inlineModel struct {
	thinking  string
	content   string
	inThink   bool
	showThink bool
	styleName string
	width     int
	height    int
	events    <-chan sseEvent
	done      bool
	renderer  *glamour.TermRenderer
	meta      *streamMeta
	rendered  string
}

func (m inlineModel) Init() tea.Cmd {
	return waitForSSE(m.events)
}

func (m inlineModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		if msg.String() == "ctrl+c" {
			m.done = true
			return m, tea.Quit
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		if r, err := glamour.NewTermRenderer(
			glamour.WithStandardStyle(m.styleName),
			glamour.WithWordWrap(msg.Width),
		); err == nil {
			m.renderer = r
		}

	case sseBatchMsg:
		m.done = applyBatch(msg, &m.thinking, &m.content, &m.inThink, &m.meta)
		m.rendered = renderMd(m.renderer, m.thinking, m.content, m.showThink)
		if m.done {
			return m, tea.Quit
		}
		return m, waitForSSE(m.events)
	}

	return m, nil
}

func (m inlineModel) View() tea.View {
	out := m.rendered
	if m.height > 0 {
		lines := strings.Split(out, "\n")
		if len(lines) > m.height {
			lines = lines[len(lines)-m.height:]
		}
		out = strings.Join(lines, "\n")
	}
	return tea.NewView(out)
}

// --- Run functions ---

func openTTY() (*os.File, func(), error) {
	tty, err := os.Open("/dev/tty")
	if err == nil {
		return tty, func() { tty.Close() }, nil
	}
	r, w, err := os.Pipe()
	if err != nil {
		return nil, nil, fmt.Errorf("cannot open terminal for input: %w", err)
	}
	return r, func() { r.Close(); w.Close() }, nil
}

func runInline(style string, width int, showThink, showInfo bool) error {
	r, err := glamour.NewTermRenderer(
		glamour.WithStandardStyle(style),
		glamour.WithWordWrap(width),
	)
	if err != nil {
		return err
	}

	ch := make(chan sseEvent, 256)
	go readEvents(ch)

	m := inlineModel{
		events:    ch,
		renderer:  r,
		showThink: showThink,
		styleName: style,
		width:     width,
	}

	tty, cleanup, err := openTTY()
	if err != nil {
		return err
	}
	defer cleanup()

	// Stream in alt-screen so no artifacts leak into scrollback.
	fmt.Fprint(os.Stdout, "\033[?1049h")
	p := tea.NewProgram(m, tea.WithInput(tty))
	result, err := p.Run()
	fmt.Fprint(os.Stdout, "\033[?1049l")

	if err != nil {
		return err
	}

	if im, ok := result.(inlineModel); ok && im.rendered != "" {
		fmt.Print(im.rendered)
		if showInfo && im.meta != nil {
			printMetaStderr(im.meta)
		}
	}
	return nil
}

func runTUI(style string, width int, showThink, showInfo bool) error {
	r, err := glamour.NewTermRenderer(
		glamour.WithStandardStyle(style),
		glamour.WithWordWrap(width),
	)
	if err != nil {
		return err
	}

	ch := make(chan sseEvent, 256)
	go readEvents(ch)

	m := tuiModel{
		viewport:   viewport.New(),
		events:     ch,
		renderer:   r,
		showThink:  showThink,
		showInfo:   showInfo,
		styleName:  style,
		autoScroll: true,
	}

	tty, cleanup, err := openTTY()
	if err != nil {
		return err
	}
	defer cleanup()

	p := tea.NewProgram(m, tea.WithInput(tty))
	_, err = p.Run()
	return err
}

func printMetaStderr(m *streamMeta) {
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
	if len(parts) > 0 {
		fmt.Fprintf(os.Stderr, "\033[2m%s\033[0m\n", strings.Join(parts, "  |  "))
	}
}

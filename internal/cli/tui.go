package cli

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type viewMode int

const (
	viewList viewMode = iota
	viewDetail
	viewLogs
	viewExec
	viewExecResult
	viewConfirmDelete
)

var (
	titleStyle  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("213"))
	headerStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("245"))
	cursorStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("213")).Bold(true)
	selRowStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("231")).Background(lipgloss.Color("57"))
	helpStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
	errStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("203")).Bold(true)
	okStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("84"))
	dimStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	urlStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("45")).Underline(true) // cyan, eye-catch
	svcStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("213")).Bold(true)      // service label
)


type listMsg []Sandbox
type detailMsg Sandbox
type logsMsg string
type execMsg ExecResult
type actionMsg string 
type errMsg struct{ err error }

func (e errMsg) Error() string { return e.err.Error() }


type Model struct {
	client *Client

	mode    viewMode
	loading bool
	status  string 
	err     string

	sandboxes []Sandbox
	cursor    int

	detail Sandbox
	logs   string
	exec   ExecResult

	input textinput.Model 

	width, height int
}

func NewModel(c *Client) Model {
	ti := textinput.New()
	ti.Placeholder = "command e.g. ls -la /"
	ti.CharLimit = 512
	ti.Prompt = "$ "
	return Model{
		client:  c,
		mode:    viewList,
		loading: true,
		input:   ti,
	}
}

func (m Model) Init() tea.Cmd {
	return m.fetchList()
}


func ctx() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), 30*time.Second)
}

func (m Model) fetchList() tea.Cmd {
	return func() tea.Msg {
		c, cancel := ctx()
		defer cancel()
		list, err := m.client.List(c)
		if err != nil {
			return errMsg{err}
		}
		return listMsg(list)
	}
}

func (m Model) fetchDetail(id string) tea.Cmd {
	return func() tea.Msg {
		c, cancel := ctx()
		defer cancel()
		s, err := m.client.Get(c, id)
		if err != nil {
			return errMsg{err}
		}
		return detailMsg(s)
	}
}

func (m Model) fetchLogs(id string) tea.Cmd {
	return func() tea.Msg {
		c, cancel := ctx()
		defer cancel()
		r, err := m.client.Logs(c, id, 200)
		if err != nil {
			return errMsg{err}
		}
		return logsMsg(r.Logs)
	}
}

func (m Model) create() tea.Cmd {
	return func() tea.Msg {
		c, cancel := ctx()
		defer cancel()
		s, err := m.client.Create(c)
		if err != nil {
			return errMsg{err}
		}
		return actionMsg("sandbox created: " + s.ID)
	}
}

func (m Model) remove(id string) tea.Cmd {
	return func() tea.Msg {
		c, cancel := ctx()
		defer cancel()
		if err := m.client.Remove(c, id); err != nil {
			return errMsg{err}
		}
		return actionMsg("sandbox removed: " + id)
	}
}

func (m Model) changeState(id, state string) tea.Cmd {
	return func() tea.Msg {
		c, cancel := ctx()
		defer cancel()
		if err := m.client.ChangeState(c, id, state); err != nil {
			return errMsg{err}
		}
		past := map[string]string{"start": "started", "stop": "stopped"}[state]
		return actionMsg("sandbox " + past + ": " + id)
	}
}

func (m Model) runExec(id string, cmd []string) tea.Cmd {
	return func() tea.Msg {
		c, cancel := ctx()
		defer cancel()
		r, err := m.client.Exec(c, id, cmd)
		if err != nil {
			return errMsg{err}
		}
		return execMsg(r)
	}
}


func (m Model) selected() (Sandbox, bool) {
	if m.cursor >= 0 && m.cursor < len(m.sandboxes) {
		return m.sandboxes[m.cursor], true
	}
	return Sandbox{}, false
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		return m, nil

	case listMsg:
		m.loading = false
		m.sandboxes = []Sandbox(msg)
		if m.cursor >= len(m.sandboxes) {
			m.cursor = max(0, len(m.sandboxes)-1)
		}
		return m, nil

	case detailMsg:
		m.loading = false
		m.detail = Sandbox(msg)
		m.mode = viewDetail
		return m, nil

	case logsMsg:
		m.loading = false
		m.logs = string(msg)
		m.mode = viewLogs
		return m, nil

	case execMsg:
		m.loading = false
		m.exec = ExecResult(msg)
		m.mode = viewExecResult
		return m, nil

	case actionMsg:
		m.loading = true
		m.status = string(msg)
		m.err = ""
		m.mode = viewList
		return m, m.fetchList()

	case errMsg:
		m.loading = false
		m.err = msg.Error()
		return m, nil

	case tea.KeyMsg:
		return m.handleKey(msg)
	}

	if m.mode == viewExec {
		var cmd tea.Cmd
		m.input, cmd = m.input.Update(msg)
		return m, cmd
	}
	return m, nil
}

func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// exec input mode captures most keys
	if m.mode == viewExec {
		switch msg.String() {
		case "esc":
			m.mode = viewList
			m.input.Blur()
			return m, nil
		case "enter":
			s, ok := m.selected()
			fields := strings.Fields(m.input.Value())
			if !ok || len(fields) == 0 {
				m.mode = viewList
				m.input.Blur()
				return m, nil
			}
			m.loading = true
			m.input.Blur()
			return m, m.runExec(s.ID, fields)
		}
		var cmd tea.Cmd
		m.input, cmd = m.input.Update(msg)
		return m, cmd
	}

	switch msg.String() {
	case "ctrl+c", "q":
		return m, tea.Quit
	}

	if m.mode == viewDetail || m.mode == viewLogs || m.mode == viewExecResult {
		switch msg.String() {
		case "esc", "enter", "backspace":
			m.mode = viewList
		}
		return m, nil
	}

	if m.mode == viewConfirmDelete {
		switch msg.String() {
		case "y", "Y":
			s, ok := m.selected()
			m.mode = viewList
			if !ok {
				return m, nil
			}
			m.loading = true
			return m, m.remove(s.ID)
		default:
			m.mode = viewList
			return m, nil
		}
	}

	// list mode
	switch msg.String() {
	case "up", "k":
		if m.cursor > 0 {
			m.cursor--
		}
	case "down", "j":
		if m.cursor < len(m.sandboxes)-1 {
			m.cursor++
		}
	case "r":
		m.loading = true
		m.err = ""
		return m, m.fetchList()
	case "c":
		m.loading = true
		m.err = ""
		return m, m.create()
	case "enter":
		if s, ok := m.selected(); ok {
			m.loading = true
			return m, m.fetchDetail(s.ID)
		}
	case "l":
		if s, ok := m.selected(); ok {
			m.loading = true
			return m, m.fetchLogs(s.ID)
		}
	case "s":
		if s, ok := m.selected(); ok {
			m.loading = true
			return m, m.changeState(s.ID, "start")
		}
	case "x":
		if s, ok := m.selected(); ok {
			m.loading = true
			return m, m.changeState(s.ID, "stop")
		}
	case "e":
		if _, ok := m.selected(); ok {
			m.mode = viewExec
			m.input.SetValue("")
			m.input.Focus()
			return m, textinput.Blink
		}
	case "d":
		if _, ok := m.selected(); ok {
			m.mode = viewConfirmDelete
		}
	}
	return m, nil
}


func (m Model) View() string {
	var b strings.Builder

	b.WriteString(titleStyle.Render("paddock") + dimStyle.Render("  —  sandbox control") + "\n\n")

	switch m.mode {
	case viewDetail:
		b.WriteString(m.viewDetail())
	case viewLogs:
		b.WriteString(m.viewLogs())
	case viewExec:
		b.WriteString(m.viewExec())
	case viewExecResult:
		b.WriteString(m.viewExecResult())
	case viewConfirmDelete:
		b.WriteString(m.viewList())
		s, _ := m.selected()
		b.WriteString("\n" + errStyle.Render(fmt.Sprintf("Delete sandbox %s? (y/N)", s.ID)))
	default:
		b.WriteString(m.viewList())
	}

	b.WriteString("\n")
	if m.loading {
		b.WriteString(dimStyle.Render("loading…") + "\n")
	}
	if m.err != "" {
		b.WriteString(errStyle.Render("error: "+m.err) + "\n")
	} else if m.status != "" {
		b.WriteString(okStyle.Render("✓ "+m.status) + "\n")
	}
	return b.String()
}

func (m Model) viewList() string {
	var b strings.Builder

	if len(m.sandboxes) == 0 && !m.loading {
		b.WriteString(dimStyle.Render("no sandboxes. press 'c' to create one.") + "\n")
	} else {
		b.WriteString("  " + headerStyle.Render(fmt.Sprintf("%-22s %-12s %-20s %s", "ID", "STATE", "IMAGE", "CREATED")) + "\n")
		for i, s := range m.sandboxes {
			row := fmt.Sprintf("%-22s %-12s %-20s %s",
				truncate(s.ID, 22), truncate(s.State, 12), truncate(s.Image, 20),
				s.Created.Format("2006-01-02 15:04"))
			if i == m.cursor {
				b.WriteString(cursorStyle.Render("▸ ") + selRowStyle.Render(row) + "\n")
			} else {
				b.WriteString("  " + row + "\n")
			}
		}
	}

	b.WriteString("\n" + helpStyle.Render(
		"↑/↓ move • enter detail • c create • s start • x stop • e exec • l logs • d delete • r refresh • q quit"))
	return b.String()
}

func (m Model) viewDetail() string {
	s := m.detail
	lastExec := "—"
	if !s.LastExec.IsZero() {
		lastExec = s.LastExec.Format(time.RFC3339)
	}
	rows := [][2]string{
		{"ID", s.ID},
		{"Name", s.Name},
		{"State", s.State},
		{"Image", s.Image},
		{"Container", s.ContainerId},
		{"Engine", s.Engine},
		{"Network", s.NetworkId},
		{"Volume", s.VolumeName},
		{"Created", s.Created.Format(time.RFC3339)},
		{"Last exec", lastExec},
	}
	var b strings.Builder
	b.WriteString(headerStyle.Render("Sandbox detail") + "\n\n")
	for _, r := range rows {
		b.WriteString("  " + dimStyle.Width(12).Render(r[0]) + " " + r[1] + "\n")
	}

	// Endpoints — colored URLs so they stand out.
	b.WriteString("\n  " + headerStyle.Render("Endpoints") + "\n")
	b.WriteString(portLine("Terminal", "http://localhost:"+s.Ports.Terminal, s.Ports.Terminal))
	b.WriteString(portLine("VNC", "http://localhost:"+s.Ports.VNC+"/vnc.html", s.Ports.VNC))
	b.WriteString(portLine("CDP", "http://localhost:"+s.Ports.CDP+"/json/version", s.Ports.CDP))

	b.WriteString("\n" + helpStyle.Render("esc/enter back"))
	return b.String()
}

func portLine(svc, url, port string) string {
	label := svcStyle.Width(10).Render(svc)
	if port == "" || port == "0" {
		return "  " + label + " " + dimStyle.Render("— (not bound)") + "\n"
	}
	return "  " + label + " " + urlStyle.Render(url) + "\n"
}

func (m Model) viewLogs() string {
	body := m.logs
	if strings.TrimSpace(body) == "" {
		body = dimStyle.Render("(empty)")
	}
	return headerStyle.Render("Logs") + "\n\n" + body + "\n\n" + helpStyle.Render("esc/enter back")
}

func (m Model) viewExec() string {
	s, _ := m.selected()
	return headerStyle.Render("Exec in "+s.ID) + "\n\n" +
		m.input.View() + "\n\n" +
		helpStyle.Render("enter run • esc cancel")
}

func (m Model) viewExecResult() string {
	var b strings.Builder
	b.WriteString(headerStyle.Render(fmt.Sprintf("Exec result (exit %d)", m.exec.ExitCode)) + "\n\n")
	if m.exec.Stdout != "" {
		b.WriteString(dimStyle.Render("stdout:") + "\n" + m.exec.Stdout + "\n")
	}
	if m.exec.Stderr != "" {
		b.WriteString(errStyle.Render("stderr:") + "\n" + m.exec.Stderr + "\n")
	}
	b.WriteString("\n" + helpStyle.Render("esc/enter back"))
	return b.String()
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	if n <= 1 {
		return s[:n]
	}
	return s[:n-1] + "…"
}

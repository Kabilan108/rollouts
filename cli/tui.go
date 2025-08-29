package main

import (
	"fmt"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// RunTUI launches an interactive TUI to collect required fields.
// Returns the completed AppConfig, a boolean indicating whether the user
// finished (true) or canceled (false), and an error if one occurred.
func RunTUI(initial AppConfig) (AppConfig, bool, error) {
	m := newTUIModel(initial)
	p := tea.NewProgram(m)
	res, err := p.Run()
	if err != nil {
		return AppConfig{}, false, err
	}
	final := res.(tuiModel)
	return final.config, final.finished, nil
}

type tuiField string

const (
	fieldName       tuiField = "name"
	fieldImage      tuiField = "image"
	fieldDomain     tuiField = "domain"
	fieldSubdomain  tuiField = "subdomain"
	fieldPort       tuiField = "port"
	fieldNetwork    tuiField = "network"
	fieldSecretMode tuiField = "secret_mode"
	fieldEnvFile    tuiField = "env_file"
	fieldMounts     tuiField = "mounts"
)

type tuiModel struct {
	input      textinput.Model
	config     AppConfig
	fields     []tuiField
	current    int
	finished   bool
	err        string
	secretMode string // none | file | edit
}

func newTUIModel(initial AppConfig) tuiModel {
	fields := []tuiField{
		fieldName,
		fieldImage,
		fieldDomain,
		fieldSubdomain,
		fieldPort,
		fieldNetwork,
		fieldSecretMode,
		fieldEnvFile, // only used when secretMode == file (otherwise skipped)
		fieldMounts,
	}

	ti := textinput.New()
	ti.Focus()
	ti.Prompt = ""
	ti.CharLimit = 200
	ti.SetValue("")

	// establish defaults for optional fields
	if initial.Network == "" {
		initial.Network = "web"
	}

	return tuiModel{
		input:   ti,
		config:  initial,
		fields:  fields,
		current: 0,
	}
}

func (m tuiModel) Init() tea.Cmd {
	return textinput.Blink
}

type tickMsg time.Time

func tickCmd() tea.Cmd {
	return tea.Tick(time.Millisecond*100, func(t time.Time) tea.Msg { return tickMsg(t) })
}

func (m tuiModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "esc":
			return m, tea.Quit
		case "enter":
			value := strings.TrimSpace(m.input.Value())
			m.err = ""

			switch m.fields[m.current] {
			case fieldName:
				if value == "" {
					m.err = "Project name is required"
					return m, nil
				}
				m.config.Name = value
			case fieldImage:
				if value == "" {
					m.err = "Docker image is required"
					return m, nil
				}
				m.config.Image = value
			case fieldDomain:
				if value == "" {
					m.err = "Domain is required"
					return m, nil
				}
				m.config.Domain = value
			case fieldSubdomain:
				m.config.Subdomain = value // optional
			case fieldPort:
				if value == "" {
					value = "80"
				}
				if port, err := strconv.Atoi(value); err == nil && port > 0 && port < 65536 {
					m.config.Port = port
				} else {
					m.err = "Enter a valid port (1-65535)"
					return m, nil
				}
			case fieldNetwork:
				if value == "" {
					// keep existing default
				} else {
					m.config.Network = value
				}
			case fieldSecretMode:
				v := strings.ToLower(value)
				if v == "" {
					v = "none"
				}
				switch v {
				case "none":
					m.secretMode = "none"
					m.config.EditEnv = false
					m.config.EnvFile = ""
				case "file":
					m.secretMode = "file"
					m.config.EditEnv = false
				case "edit":
					m.secretMode = "edit"
					m.config.EditEnv = true
					m.config.EnvFile = ""
				default:
					m.err = "Choose one: none, file, or edit"
					return m, nil
				}
			case fieldEnvFile:
				if m.secretMode == "file" {
					if value == "" {
						m.err = "Provide a file path or choose a different secret mode"
						return m, nil
					}
					m.config.EnvFile = value
				}
				// if mode is not file, skip
			case fieldMounts:
				if value == "" {
					m.config.Mounts = nil
				} else {
					parts := strings.Split(value, ",")
					out := make([]string, 0, len(parts))
					for _, p := range parts {
						p = strings.TrimSpace(p)
						if p != "" {
							out = append(out, p)
						}
					}
					m.config.Mounts = out
				}
			}

			// next field or finish
			// skip env file prompt if not needed
			if m.fields[m.current] == fieldSecretMode && m.secretMode != "file" {
				// advance directly to next and skip env file field
				m.current++ // move to fieldEnvFile
				m.current++ // move past env file
			} else {
				m.current++
			}
			m.input.SetValue("")
			if m.current >= len(m.fields) {
				m.finished = true
				return m, tea.Quit
			}
			return m, textinput.Blink
		}
	}

	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

func (m tuiModel) View() string {
	if m.finished || m.current >= len(m.fields) {
		return ""
	}

	var b strings.Builder
	b.WriteString(headerStyle.Render("Rollout Init") + "\n")
	b.WriteString(subHeaderStyle.Render(fmt.Sprintf("Step %d of %d", m.current+1, len(m.fields))) + "\n\n")

	var prompt, placeholder, help string
	switch m.fields[m.current] {
	case fieldName:
		prompt = "Project Name"
		placeholder = "my-awesome-app"
	case fieldImage:
		prompt = "Docker Image"
		placeholder = "nginx:latest"
	case fieldDomain:
		prompt = "Main Domain"
		placeholder = "example.com"
	case fieldSubdomain:
		prompt = "Subdomain (optional)"
		placeholder = "api"
	case fieldPort:
		prompt = "Container Port"
		placeholder = "80"
	case fieldNetwork:
		prompt = "Network"
		placeholder = m.config.Network
	case fieldSecretMode:
		prompt = "Secrets (none | file | edit)"
		placeholder = "none"
		help = "file: path to env file, edit: open editor"
	case fieldEnvFile:
		prompt = "Environment file path"
		placeholder = "/path/to/.env"
	case fieldMounts:
		prompt = "Mounts (comma-separated)"
		placeholder = "/host:/container:rw, name:/container:ro"
	}

	m.input.Placeholder = placeholder
	// Ensure the full placeholder is visible: set width to at least placeholder length
	width := utf8.RuneCountInString(placeholder)
	if width < 20 {
		width = 20
	}
	m.input.Width = width
	// Use accent color for prompts to balance UI colors
	tuiPromptStyle := lipgloss.NewStyle().Foreground(accentColor).Bold(true)
	b.WriteString(tuiPromptStyle.Render(prompt) + "\n")
	b.WriteString(inputStyle.Render(m.input.View()) + "\n")
	if help != "" {
		b.WriteString(mutedStyle.Render(help) + "\n")
	}
	if m.err != "" {
		b.WriteString(errorStyle.Render("Error: "+m.err) + "\n")
	}
	b.WriteString(mutedStyle.Render("Press Enter to continue, Ctrl+C to cancel"))
	return b.String()
}

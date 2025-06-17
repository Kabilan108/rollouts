package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"
)

var (
	promptStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#EDB73A"))

	inputStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#374151"))

	errorStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#DC2626"))

	successStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#059669"))
)

type NixAppConfig struct {
	Name          string
	Image         string
	ContainerPort int
	Domain        string
	Subdomain     string
	Network       string
}

func (c *NixAppConfig) Generate() string {
	var hostRule string
	if c.Subdomain != "" {
		subdomainHost := fmt.Sprintf("%s.%s", c.Subdomain, c.Domain)
		hostRule = fmt.Sprintf("Host(`%s`) || Host(`www.%s`)", subdomainHost, subdomainHost)
	} else {
		hostRule = fmt.Sprintf("Host(`%s`) || Host(`www.%s`)", c.Domain, c.Domain)
	}

	nixTemplate := `{
  virtualisation.oci-containers.containers."%s" = rec {
    image = "%s";
    ports = [ "127.0.0.1:%d:%d" ];
    networks = [ "%s" ];
    labels = {
      "traefik.enable" = "true";
      "traefik.docker.network" = "%s";
      "traefik.http.services.%s.loadbalancer.server.port" = "%d";

      # domain router
      "traefik.http.routers.%s.rule" = "%s";
      "traefik.http.routers.%s.entrypoints" = "websecure";
      "traefik.http.routers.%s.tls.certresolver" = "letsencrypt";
    };
  };
}`

	hostPort := c.ContainerPort + 10000

	return fmt.Sprintf(nixTemplate,
		c.Name,
		c.Image,
		hostPort, c.ContainerPort,
		c.Network,
		c.Network,
		c.Name, c.ContainerPort,
		c.Name, hostRule,
		c.Name,
		c.Name,
	)
}

type AppConfig struct {
	Name      string
	Image     string
	Domain    string
	Subdomain string
	Port      int
	ConfigDir string
	Network   string
	DryRun    bool
}

type model struct {
	input        textinput.Model
	config       AppConfig
	fields       []string
	current      int
	finished     bool
	interactions []string
}

func initialModel(config AppConfig) model {
	fields := []string{}

	if config.Name == "" {
		fields = append(fields, "name")
	}
	if config.Image == "" {
		fields = append(fields, "image")
	}
	if config.Domain == "" {
		fields = append(fields, "domain")
	}
	if config.Port == 0 {
		fields = append(fields, "port")
	}
	if len(fields) > 0 && config.Subdomain == "" {
		fields = append(fields, "subdomain")
	}

	ti := textinput.New()
	ti.Focus()

	return model{
		input:        ti,
		config:       config,
		fields:       fields,
		current:      0,
		interactions: []string{},
	}
}

func (m model) Init() tea.Cmd {
	return textinput.Blink
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "esc":
			return m, tea.Quit
		case "enter":
			value := m.input.Value()

			var prompt string
			switch m.fields[m.current] {
			case "name":
				prompt = "Project Name: "
				m.config.Name = value
			case "image":
				prompt = "Docker Image URL: "
				m.config.Image = value
			case "domain":
				prompt = "Main Domain: "
				m.config.Domain = value
			case "subdomain":
				prompt = "Subdomain (optional): "
				m.config.Subdomain = value
			case "port":
				prompt = "Container Port: "
				if value != "" {
					if port, err := strconv.Atoi(value); err == nil {
						m.config.Port = port
					}
				}
			}

			m.interactions = append(m.interactions, prompt+value)

			m.current++
			if m.current >= len(m.fields) {
				m.finished = true
				return m, tea.Quit
			}

			m.input.SetValue("")
			return m, textinput.Blink
		}
	}

	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

func (m model) View() string {
	if len(m.fields) == 0 || m.current >= len(m.fields) {
		return ""
	}

	var output strings.Builder

	for _, interaction := range m.interactions {
		output.WriteString(interaction + "\n")
	}

	var prompt string
	switch m.fields[m.current] {
	case "name":
		prompt = "Project Name: "
	case "image":
		prompt = "Docker Image URL: "
	case "domain":
		prompt = "Main Domain: "
	case "subdomain":
		prompt = "Subdomain (optional): "
	case "port":
		prompt = "Container Port: "
	}

	output.WriteString(promptStyle.Render(prompt) + inputStyle.Render(m.input.View()))

	return output.String()
}

func printGitHubAction(branch string) {
	yaml := `name: Build and Push to GitHub Container Registry

on:
  workflow_dispatch:
  push:
    branches: [%s]

env:
  REGISTRY: ghcr.io
  IMAGE_NAME: ${{ github.repository }}

jobs:
  build-and-push:
    runs-on: ubuntu-latest
    permissions:
      contents: read
      packages: write

    steps:
      - name: Checkout repository
        run: |
          git clone https://github.com/${{ github.repository }}.git .
          git checkout ${{ github.sha }}

      - name: Setup Docker environment
        run: |
          # convert repository name to lowercase for container registry
          echo "IMAGE_NAME_LOWER=$(echo ${{ env.IMAGE_NAME }} | tr '[:upper:]' '[:lower:]')" >> $GITHUB_ENV
          echo "FULL_IMAGE_NAME=${{ env.REGISTRY }}/$(echo ${{ env.IMAGE_NAME }} | tr '[:upper:]' '[:lower:]')" >> $GITHUB_ENV

      - name: Login to GitHub Container Registry
        run: |
          echo "${{ secrets.GITHUB_TOKEN }}" | docker login ${{ env.REGISTRY }} -u ${{ github.actor }} --password-stdin

      - name: Build Docker image
        run: |
          docker build -t ${{ env.FULL_IMAGE_NAME }}:latest .
          docker tag ${{ env.FULL_IMAGE_NAME }}:latest ${{ env.FULL_IMAGE_NAME }}:${{ github.sha }}

      - name: Push Docker image
        run: |
          docker push ${{ env.FULL_IMAGE_NAME }}:latest
          docker push ${{ env.FULL_IMAGE_NAME }}:${{ github.sha }}

      - name: Output image details
        run: |
          echo "Successfully pushed Docker image:"
          echo "- ${{ env.FULL_IMAGE_NAME }}:latest"
          echo "- ${{ env.FULL_IMAGE_NAME }}:${{ github.sha }}"
          echo ""
          echo "To pull this image:"
          echo "docker pull ${{ env.FULL_IMAGE_NAME }}:latest"

      - name: Triegger redeploy
        env:
          DEPLOY_PAT: ${{ secrets.DEPLOY_PAT }}
        run: |
          curl -L -X POST \
            -H "Accept: application/vnd.github+json" \
            -H "Authorization: Bearer $DEPLOY_PAT" \
            -H "X-GitHub-Api-Version: 2022-11-28" \
            https://api.github.com/repos/Kabilan108/dotfiles/dispatches \
            -d '{"event_type":"deploy"}'`

	fmt.Fprintln(os.Stderr, promptStyle.Render("remember to run `gh secret set DEPLOY_PAT --body <TOKEN>` to set the necessary secrets"))
	fmt.Fprintf(os.Stdout, yaml, branch)
}

func main() {
	var configDir string
	defaultConfDir := filepath.Join(os.Getenv("HOME"), "repos", "rollouts", "servers")

	rootCmd := &cobra.Command{
		Use:   "rollout",
		Short: "rollout - nix config generator for oci-containers with traefik",
	}

	rootCmd.PersistentFlags().StringVar(&configDir, "config-dir", defaultConfDir, "path to config directory")

	var (
		name      string
		image     string
		domain    string
		subdomain string
		port      int
		network   string
		dryRun    bool
		branch    string
	)

	initCmd := &cobra.Command{
		Use:   "init",
		Short: "create a new nix config for an app",
		Run: func(cmd *cobra.Command, args []string) {
			c := AppConfig{
				Name:      name,
				Image:     image,
				Domain:    domain,
				Subdomain: subdomain,
				Port:      port,
				ConfigDir: configDir,
				Network:   network,
				DryRun:    dryRun,
			}
			runInitWithAppConfig(c)
		},
	}

	initCmd.Flags().StringVar(&name, "name", "", "project name (e.g., kabilan108-com)")
	initCmd.Flags().StringVar(&image, "image", "", "docker image url (e.g., ghcr.io/kabilan108/kabilan108.com:latest)")
	initCmd.Flags().StringVar(&domain, "domain", "", "main domain (e.g., kabilan108.com)")
	initCmd.Flags().StringVar(&subdomain, "subdomain", "", "subdomain (leave blank for none)")
	initCmd.Flags().IntVar(&port, "port", 80, "port the container exposes (e.g., 80)")
	initCmd.Flags().StringVar(&network, "network", "web", "traefik docker network")
	initCmd.Flags().BoolVar(&dryRun, "dry-run", false, "print out the generated config but don't write it to disk")

	ghActionCmd := &cobra.Command{
		Use:   "gh-action",
		Short: "print GitHub Actions workflow for container deployment",
		Run: func(cmd *cobra.Command, args []string) {
			printGitHubAction(branch)
		},
	}
	ghActionCmd.Flags().StringVar(&branch, "branch", "main", "branch to deploy from")

	rootCmd.AddCommand(initCmd)
	rootCmd.AddCommand(ghActionCmd)
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func runInitWithAppConfig(app AppConfig) {
	m := initialModel(app)

	if len(m.fields) == 0 {
		generateAndWriteConfig(app)
		return
	}

	p := tea.NewProgram(m)
	finalModel, err := p.Run()
	if err != nil {
		fmt.Println(errorStyle.Render("Error: " + err.Error()))
		os.Exit(1)
	}

	if finalModel.(model).finished {
		generateAndWriteConfig(finalModel.(model).config)
	}
}

func generateAndWriteConfig(app AppConfig) {
	config := NixAppConfig{
		Name:          app.Name,
		Image:         app.Image,
		Domain:        app.Domain,
		Subdomain:     app.Subdomain,
		ContainerPort: app.Port,
		Network:       app.Network,
	}

	nixConfig := config.Generate()
	fmt.Println(successStyle.Render("Generated config:"))
	fmt.Println(nixConfig)

	if app.DryRun {
		return
	}

	filePath := filepath.Join(app.ConfigDir, "apps", fmt.Sprintf("%s.nix", config.Name))

	err := os.WriteFile(filePath, []byte(nixConfig), 0o644)
	if err != nil {
		fmt.Println(errorStyle.Render("Failed to write file: " + err.Error()))
		os.Exit(1)
	}
	fmt.Println(successStyle.Render("Written to " + filePath))
}

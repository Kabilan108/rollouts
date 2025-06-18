package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/progress"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"
)

var (
	// Catppuccin Mocha Colors
	primaryColor  = lipgloss.Color("#cba6f7") // mauve
	accentColor   = lipgloss.Color("#f9e2af") // yellow
	mutedColor    = lipgloss.Color("#a6adc8") // subtext0
	inactiveColor = lipgloss.Color("#45475a") // surface1
	errorColor    = lipgloss.Color("#f38ba8") // red
	successColor  = lipgloss.Color("#a6e3a1") // green
	bgColor       = lipgloss.Color("#1e1e2e") // base
	borderColor   = lipgloss.Color("#45475a") // surface1
	textColor     = lipgloss.Color("#cdd6f4") // text

	// Styles
	headerStyle = lipgloss.NewStyle().
			Foreground(primaryColor).
			Bold(true)

	subHeaderStyle = lipgloss.NewStyle().
			Foreground(mutedColor)

	promptStyle = lipgloss.NewStyle().
			Foreground(primaryColor).
			Bold(true)

	inputStyle = lipgloss.NewStyle().
			Foreground(textColor)

	completedStyle = lipgloss.NewStyle().
			Foreground(inactiveColor)

	errorStyle = lipgloss.NewStyle().
			Foreground(errorColor).
			Bold(true)

	successStyle = lipgloss.NewStyle().
			Foreground(successColor).
			Bold(true)

	boxStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(borderColor).
			Padding(1, 2).
			Margin(0, 0, 1, 0)

	mutedStyle = lipgloss.NewStyle().
			Foreground(mutedColor)
)

type NixAppConfig struct {
	Name          string
	Image         string
	ContainerPort int
	Domain        string
	Subdomain     string
	Network       string
	HasSecrets    bool
}

func (c *NixAppConfig) Generate() string {
	var hostRule string
	if c.Subdomain != "" {
		subdomainHost := fmt.Sprintf("%s.%s", c.Subdomain, c.Domain)
		hostRule = fmt.Sprintf("Host(`%s`) || Host(`www.%s`)", subdomainHost, subdomainHost)
	} else {
		hostRule = fmt.Sprintf("Host(`%s`) || Host(`www.%s`)", c.Domain, c.Domain)
	}

	nixTemplate := `{ config, ... }:
{
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
    };%s
  };%s
}`

	// BUG: tis will collide quickly. figure out a better way to do this
	hostPort := c.ContainerPort + 10000

	var envFileAttr, ageSecretAttr string
	if c.HasSecrets {
		envFileAttr = fmt.Sprintf(`
    environmentFiles = [ config.age.secrets."%s".path ];`, c.Name)
		ageSecretAttr = fmt.Sprintf(`
  age.secrets."%s".file = ./%s.age;`, c.Name, c.Name)
	}

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
		envFileAttr,
		ageSecretAttr,
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
	EnvFile   string
	EditEnv   bool
}

type model struct {
	input        textinput.Model
	config       AppConfig
	fields       []string
	current      int
	finished     bool
	interactions []string
	progress     progress.Model
	errorMsg     string
	validating   bool
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
	ti.Prompt = ""
	ti.CharLimit = 100

	prog := progress.New(progress.WithDefaultGradient())
	prog.Width = 40

	return model{
		input:        ti,
		config:       config,
		fields:       fields,
		current:      0,
		interactions: []string{},
		progress:     prog,
	}
}

func (m model) Init() tea.Cmd {
	return textinput.Blink
}

type tickMsg time.Time

func tickCmd() tea.Cmd {
	return tea.Tick(time.Millisecond*100, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "esc":
			return m, tea.Quit
		case "enter":
			value := strings.TrimSpace(m.input.Value())
			m.errorMsg = ""

			// Validation
			if value == "" && m.fields[m.current] != "subdomain" {
				m.errorMsg = "This field is required"
				return m, nil
			}

			var prompt, displayValue string
			switch m.fields[m.current] {
			case "name":
				prompt = "Project Name"
				m.config.Name = value
				displayValue = value
			case "image":
				prompt = "Docker Image"
				m.config.Image = value
				displayValue = value
			case "domain":
				prompt = "Main Domain"
				m.config.Domain = value
				displayValue = value
			case "subdomain":
				prompt = "Subdomain"
				m.config.Subdomain = value
				if value == "" {
					displayValue = "(none)"
				} else {
					displayValue = value
				}
			case "port":
				prompt = "Container Port"
				if port, err := strconv.Atoi(value); err == nil && port > 0 && port < 65536 {
					m.config.Port = port
					displayValue = value
				} else {
					m.errorMsg = "Please enter a valid port number (1-65535)"
					return m, nil
				}
			}

			m.interactions = append(m.interactions, fmt.Sprintf("%s: %s", prompt, displayValue))

			m.current++
			if m.current >= len(m.fields) {
				m.finished = true
				return m, tea.Quit
			}

			m.input.SetValue("")
			return m, textinput.Blink
		}
	case tickMsg:
		if m.validating {
			return m, tickCmd()
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

	// Header
	header := headerStyle.Render("🚀 Rollout Configuration")
	subHeader := subHeaderStyle.Render("Generate NixOS container configs with Traefik")
	output.WriteString(header + "\n" + subHeader + "\n\n")

	// Progress
	progressText := fmt.Sprintf("Step %d of %d", m.current+1, len(m.fields))
	progressPercent := float64(m.current) / float64(len(m.fields))
	progressView := m.progress.ViewAs(progressPercent)
	output.WriteString(progressView + "\n" + mutedStyle.Render(progressText) + "\n\n")

	// Previous interactions - compact
	for _, interaction := range m.interactions {
		output.WriteString(completedStyle.Render(interaction) + "\n")
	}

	// Current field
	var prompt, placeholder string
	switch m.fields[m.current] {
	case "name":
		prompt = "Project Name"
		placeholder = "my-awesome-app"
	case "image":
		prompt = "Docker Image"
		placeholder = "nginx:latest"
	case "domain":
		prompt = "Main Domain"
		placeholder = "example.com"
	case "subdomain":
		prompt = "Subdomain (optional)"
		placeholder = "api"
	case "port":
		prompt = "Container Port"
		placeholder = "80"
	}

	m.input.Placeholder = placeholder

	// Simple input
	output.WriteString(promptStyle.Render("→ "+prompt) + "\n")
	output.WriteString(inputStyle.Render(m.input.View()) + "\n")

	if m.errorMsg != "" {
		output.WriteString(errorStyle.Render("✗ "+m.errorMsg) + "\n")
	}

	output.WriteString(mutedStyle.Render("Press Enter to continue • Ctrl+C to quit"))

	return output.String()
}

func printGitHubAction(branch string) {
	fmt.Println(headerStyle.Render("🚀 GitHub Actions Workflow"))
	fmt.Println(subHeaderStyle.Render("Copy this workflow to .github/workflows/deploy.yml"))

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

	workflowBox := strings.Builder{}
	workflowBox.WriteString(mutedStyle.Render(fmt.Sprintf(yaml, branch)))
	fmt.Println(boxStyle.Render(workflowBox.String()))

	fmt.Println(promptStyle.Render("Next Steps:"))
	fmt.Println("• Save this workflow to .github/workflows/deploy.yml")
	fmt.Println("• Run: " + successStyle.Render("gh secret set DEPLOY_PAT --body <TOKEN>"))
	fmt.Println("• Push to trigger the workflow")
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
		envFile   string
		edit      bool
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
				EnvFile:   envFile,
				EditEnv:   edit,
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
	initCmd.Flags().StringVar(&envFile, "env-file", "", "path to environment file. will be encrypted with agenix")
	initCmd.Flags().BoolVar(&edit, "edit", false, "edit the environment file directly")
	initCmd.Flags().BoolVar(&dryRun, "dry-run", false, "print out the generated config but don't write it to disk")

	ghActionCmd := &cobra.Command{
		Use:   "gh-action",
		Short: "print GitHub Actions workflow for container deployment",
		Run: func(cmd *cobra.Command, args []string) {
			printGitHubAction(branch)
		},
	}
	ghActionCmd.Flags().StringVar(&branch, "branch", "main", "branch to deploy from")

	pushCmd := &cobra.Command{
		Use:   "push",
		Short: "commit and push changes to the rollouts repository",
		Run: func(cmd *cobra.Command, args []string) {
			runPushCommand()
		},
	}

	rootCmd.AddCommand(initCmd)
	rootCmd.AddCommand(ghActionCmd)
	rootCmd.AddCommand(pushCmd)
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
		if finalModel.(model).config.EditEnv {
			fmt.Println(promptStyle.Render("Opening agenix editor for " + filepath.Join("servers", "apps", fmt.Sprintf("%s.age", finalModel.(model).config.Name))))
		}
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
		HasSecrets:    app.EnvFile != "" || (app.EditEnv && app.EnvFile == ""),
	}

	nixConfig := config.Generate()

	// Configuration summary
	fmt.Println(headerStyle.Render("✨ Configuration Generated"))
	fmt.Println()

	summaryBox := strings.Builder{}
	summaryBox.WriteString(promptStyle.Render("Configuration Summary") + "\n")
	summaryBox.WriteString(fmt.Sprintf("• Name: %s\n", successStyle.Render(config.Name)))
	summaryBox.WriteString(fmt.Sprintf("• Image: %s\n", successStyle.Render(config.Image)))
	if config.Subdomain != "" {
		summaryBox.WriteString(fmt.Sprintf("• URL: %s\n", successStyle.Render(fmt.Sprintf("https://%s.%s", config.Subdomain, config.Domain))))
	} else {
		summaryBox.WriteString(fmt.Sprintf("• URL: %s\n", successStyle.Render(fmt.Sprintf("https://%s", config.Domain))))
	}
	summaryBox.WriteString(fmt.Sprintf("• Port: %s\n", successStyle.Render(fmt.Sprintf("%d", config.ContainerPort))))
	summaryBox.WriteString(fmt.Sprintf("• Network: %s", successStyle.Render(config.Network)))
	if config.HasSecrets {
		summaryBox.WriteString(fmt.Sprintf("\n• Secrets: %s", successStyle.Render("Enabled")))
	}
	fmt.Println(boxStyle.Render(summaryBox.String()))

	// Generated config
	configBox := strings.Builder{}
	configBox.WriteString(promptStyle.Render("Generated NixOS Configuration") + "\n")
	configBox.WriteString(mutedStyle.Render(nixConfig))
	fmt.Println(boxStyle.Render(configBox.String()))

	if app.DryRun {
		fmt.Println(promptStyle.Render("ℹ️ Dry run mode - no files written"))
		return
	}

	// File operations
	filePath := filepath.Join(app.ConfigDir, "apps", fmt.Sprintf("%s.nix", config.Name))
	err := os.WriteFile(filePath, []byte(nixConfig), 0o644)
	if err != nil {
		fmt.Println(errorStyle.Render("✗ Failed to write file: " + err.Error()))
		os.Exit(1)
	}
	fmt.Println(successStyle.Render("✓ Configuration written to " + filePath))

	// Handle secrets if any are needed
	if config.HasSecrets {
		secretsNixPath := filepath.Join(app.ConfigDir, "..", "secrets.nix")
		if err = updateSecretsNix(config.Name, secretsNixPath); err != nil {
			fmt.Println(errorStyle.Render("✗ Failed to update secrets.nix: " + err.Error()))
			os.Exit(1)
		}
		fmt.Println(successStyle.Render("✓ Updated " + secretsNixPath))

		if app.EditEnv {
			err = openAgenixEditor(config.Name, filepath.Join(app.ConfigDir, "apps"))
			if err != nil {
				fmt.Println(errorStyle.Render("✗ Failed to open agenix editor: " + err.Error()))
				os.Exit(1)
			}
		} else if app.EnvFile != "" {
			err = createAndEncryptSecret(app.EnvFile, config.Name, filepath.Join(app.ConfigDir, "apps"))
			if err != nil {
				fmt.Println(errorStyle.Render("✗ Failed to encrypt secrets: " + err.Error()))
				os.Exit(1)
			}
		}
	}

	fmt.Println(successStyle.Render("✨ Setup complete! Your application is ready to deploy."))
}

func updateSecretsNix(appName, secretsPath string) error {
	content, err := os.ReadFile(secretsPath)
	if err != nil {
		return err
	}

	// check if .age file exists for the app
	ageEntryPrefix := fmt.Sprintf(`"servers/apps/%s.age"`, appName)
	if strings.Contains(string(content), ageEntryPrefix) {
		fmt.Println(mutedStyle.Render("ℹ️ Skipping " + ageEntryPrefix + " (already exists)"))
		return nil
	}

	// find the public keys
	re := regexp.MustCompile(`"servers\/secrets\/system\.age"\.publicKeys = (\[[^\]]*\]);`)
	matches := re.FindSubmatch(content)
	if len(matches) < 2 {
		return fmt.Errorf("could not find system public keys in %s", secretsPath)
	}
	publicKeys := string(matches[1])

	// prepare the new entry
	newEntry := fmt.Sprintf(`
  "servers/apps/%s.age".publicKeys = %s;
`, appName, publicKeys)

	// Find the last '}' in the file and insert the new entry before it.
	lastBraceIndex := strings.LastIndex(string(content), "}")
	if lastBraceIndex == -1 {
		return fmt.Errorf("could not find closing brace in %s", secretsPath)
	}

	newContent := string(content[:lastBraceIndex]) + newEntry + string(content[lastBraceIndex:])

	return os.WriteFile(secretsPath, []byte(newContent), 0o644)
}

// this function encrypts a given environment file to the correct location
// using `agenix -e`.
func createAndEncryptSecret(sourceEnvFile, appName, appsDir string) error {
	sourceFile, err := os.Open(sourceEnvFile)
	if err != nil {
		return fmt.Errorf("could not open source env file %s: %w", sourceEnvFile, err)
	}
	defer sourceFile.Close()

	encryptedFilePath := filepath.Join("servers", "apps", fmt.Sprintf("%s.age", appName))

	fmt.Println(promptStyle.Render(fmt.Sprintf("🔐 Encrypting %s to %s", sourceEnvFile, encryptedFilePath)))

	// prepare the `agenix -e` command.
	// we run it from the repository root so agenix can find secrets.nix.
	cmd := exec.Command("agenix", "-e", encryptedFilePath)
	cmd.Dir = filepath.Join(appsDir, "..", "..")
	cmd.Stdin = sourceFile // pipe contents of source file to stdin.

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("agenix encryption command failed:\n%s", string(output))
	}

	fmt.Println(mutedStyle.Render(string(output)))
	fmt.Println(successStyle.Render("✓ Successfully encrypted secret to " + encryptedFilePath))
	return nil
}

func openAgenixEditor(appName, appsDir string) error {
	encryptedFilePath := filepath.Join("servers", "apps", fmt.Sprintf("%s.age", appName))

	fmt.Println(promptStyle.Render(fmt.Sprintf("🔐 Opening agenix editor for %s", encryptedFilePath)))

	cmd := exec.Command("agenix", "-e", encryptedFilePath)
	cmd.Dir = filepath.Join(appsDir, "..", "..")
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	err := cmd.Run()
	if err != nil {
		return fmt.Errorf("agenix editor command failed: %w", err)
	}

	fmt.Println(successStyle.Render("✓ Successfully edited secret " + encryptedFilePath))
	return nil
}

func runPushCommand() {
	repoDir := filepath.Join(os.Getenv("HOME"), "repos", "rollouts")
	
	// check if we're in the rollouts directory structure
	if wd, err := os.Getwd(); err == nil {
		if strings.Contains(wd, "rollouts") {
			// Find the rollouts root by walking up the directory tree
			dir := wd
			for {
				if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
					repoDir = dir
					break
				}
				parent := filepath.Dir(dir)
				if parent == dir {
					// Reached filesystem root, use default
					break
				}
				dir = parent
			}
		}
	}

	// Header
	fmt.Println(headerStyle.Render("🚀 Git Push Automation"))
	fmt.Println(subHeaderStyle.Render("Committing and pushing rollout changes"))
	fmt.Println()

	// Repository info
	repoBox := strings.Builder{}
	repoBox.WriteString(promptStyle.Render("Repository Information") + "\n")
	repoBox.WriteString(fmt.Sprintf("• Directory: %s\n", successStyle.Render(repoDir)))
	repoBox.WriteString(fmt.Sprintf("• Command: %s", mutedStyle.Render("git add . && git commit && git push")))
	fmt.Println(boxStyle.Render(repoBox.String()))

	// Stage changes
	fmt.Println(promptStyle.Render("→ Staging changes..."))
	addCmd := exec.Command("git", "add", ".")
	addCmd.Dir = repoDir
	if output, err := addCmd.CombinedOutput(); err != nil {
		fmt.Println(errorStyle.Render("✗ Failed to stage changes: " + err.Error()))
		if len(output) > 0 {
			fmt.Println(mutedStyle.Render(string(output)))
		}
		os.Exit(1)
	}
	fmt.Println(successStyle.Render("✓ Changes staged successfully"))

	// Commit changes
	fmt.Println(promptStyle.Render("→ Creating commit..."))
	commitCmd := exec.Command("git", "commit", "-m", "rollout: automated commit via push command")
	commitCmd.Dir = repoDir
	commitOutput, err := commitCmd.CombinedOutput()
	if err != nil {
		// check if it's just "nothing to commit"
		if strings.Contains(string(commitOutput), "nothing to commit") {
			fmt.Println(mutedStyle.Render("ℹ️ No changes to commit - repository is up to date"))
			return
		} else {
			fmt.Println(errorStyle.Render("✗ Failed to commit changes: " + err.Error()))
			if len(commitOutput) > 0 {
				fmt.Println(mutedStyle.Render(string(commitOutput)))
			}
			os.Exit(1)
		}
	} else {
		fmt.Println(successStyle.Render("✓ Commit created successfully"))
		if len(commitOutput) > 0 {
			fmt.Println(mutedStyle.Render(strings.TrimSpace(string(commitOutput))))
		}
	}

	// Push changes
	fmt.Println(promptStyle.Render("→ Pushing to remote..."))
	pushCmd := exec.Command("git", "push")
	pushCmd.Dir = repoDir
	pushOutput, err := pushCmd.CombinedOutput()
	if err != nil {
		fmt.Println(errorStyle.Render("✗ Failed to push changes: " + err.Error()))
		if len(pushOutput) > 0 {
			fmt.Println(mutedStyle.Render(string(pushOutput)))
		}
		os.Exit(1)
	}

	fmt.Println(successStyle.Render("✓ Successfully pushed to remote"))
	if len(pushOutput) > 0 {
		fmt.Println(mutedStyle.Render(strings.TrimSpace(string(pushOutput))))
	}
	
	fmt.Println()
	fmt.Println(successStyle.Render("✨ Push completed! Your changes are now live."))
}

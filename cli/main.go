package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

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

type PortRegistry struct {
	Allocations map[string]int `json:"allocations"`
	NextPort    int            `json:"next_port"`
}

type NixAppConfig struct {
	Name          string
	Image         string
	ContainerPort int
	Domain        string
	Subdomain     string
	Network       string
	HasSecrets    bool
	HostPort      int
	Mounts        []string
}

func (c *NixAppConfig) Generate() string {
	var hostRule string
	if c.Subdomain != "" {
		subdomainHost := fmt.Sprintf("%s.%s", c.Subdomain, c.Domain)
		hostRule = fmt.Sprintf("Host(`%s`) || Host(`www.%s`)", subdomainHost, subdomainHost)
	} else {
		hostRule = fmt.Sprintf("Host(`%s`) || Host(`www.%s`)", c.Domain, c.Domain)
	}

	nixTemplate := `{ config, pkgs, ... }:
{
  virtualisation.oci-containers.containers."%s" = rec {
    image = "%s";
    ports = [ "127.0.0.1:%d:%d" ];
    networks = [ "%s" ];
%s
    labels = {
      "traefik.enable" = "true";
      "traefik.docker.network" = "%s";
      "traefik.http.services.%s.loadbalancer.server.port" = "%d";

      # domain router
      "traefik.http.routers.%s.rule" = "%s";
      "traefik.http.routers.%s.entrypoints" = "websecure";
      "traefik.http.routers.%s.tls.certresolver" = "letsencrypt";
    };%s
  };

  # Force image pull on every deployment
  systemd.services."docker-%s".serviceConfig.ExecStartPre = [
    "${pkgs.docker}/bin/docker pull %s"
  ];%s
}`

	// Use the allocated host port instead of calculating it
	hostPort := c.HostPort

	var envFileAttr, ageSecretAttr string
	if c.HasSecrets {
		envFileAttr = fmt.Sprintf(`
    environmentFiles = [ config.age.secrets."%s".path ];`, c.Name)
		ageSecretAttr = fmt.Sprintf(`
  age.secrets."%s".file = ./%s.age;`, c.Name, c.Name)
	}

	// Volumes (mounts) attribute
	var volumesAttr string
	if len(c.Mounts) > 0 {
		// join mounts into Nix list of strings
		mounts := make([]string, 0, len(c.Mounts))
		for _, m := range c.Mounts {
			// pass-through without validation
			mounts = append(mounts, fmt.Sprintf("\"%s\"", m))
		}
		volumesAttr = fmt.Sprintf("    volumes = [ %s ];", strings.Join(mounts, " "))
	}

	return fmt.Sprintf(nixTemplate,
		c.Name,
		c.Image,
		hostPort, c.ContainerPort,
		c.Network,
		volumesAttr,
		c.Network,
		c.Name, c.ContainerPort,
		c.Name, hostRule,
		c.Name,
		c.Name,
		envFileAttr,
		c.Name,
		c.Image,
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
	Mounts    []string
}

// AppConfig holds the configuration fields for an app

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
            https://api.github.com/repos/Kabilan108/rollouts/dispatches \
            -d '{"event_type":"deploy"}'`

	// Print raw YAML to stdout
	fmt.Printf(yaml, branch)

	// Print styled messages to stderr
	fmt.Fprintln(os.Stderr, headerStyle.Render("🚀 GitHub Actions Workflow"))
	fmt.Fprintln(os.Stderr, subHeaderStyle.Render("Copy this workflow to .github/workflows/deploy.yml"))
	fmt.Fprintln(os.Stderr)
	fmt.Fprintln(os.Stderr, promptStyle.Render("Next Steps:"))
	fmt.Fprintln(os.Stderr, "• Save this workflow to .github/workflows/deploy.yml")
	fmt.Fprintln(os.Stderr, "• Run: "+successStyle.Render("gh secret set DEPLOY_PAT --body <TOKEN>"))
	fmt.Fprintln(os.Stderr, "• Push to trigger the workflow")
}

const (
	// Port allocation range: 10000-19999 (10,000 ports available)
	PortRangeStart = 10000
	PortRangeEnd   = 19999
)

func loadPortRegistry(configDir string) (*PortRegistry, error) {
	registryPath := filepath.Join(configDir, "ports.json")

	registry := &PortRegistry{
		Allocations: make(map[string]int),
		NextPort:    PortRangeStart,
	}

	data, err := os.ReadFile(registryPath)
	if err != nil {
		if os.IsNotExist(err) {
			// Registry doesn't exist, scan existing apps to build it
			if err := initializeRegistryFromExistingApps(registry, configDir); err != nil {
				return nil, fmt.Errorf("failed to initialize registry from existing apps: %w", err)
			}
			return registry, nil
		}
		return nil, fmt.Errorf("failed to read port registry: %w", err)
	}

	if err := json.Unmarshal(data, registry); err != nil {
		return nil, fmt.Errorf("failed to parse port registry: %w", err)
	}

	return registry, nil
}

func savePortRegistry(registry *PortRegistry, configDir string) error {
	registryPath := filepath.Join(configDir, "ports.json")

	data, err := json.MarshalIndent(registry, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal port registry: %w", err)
	}

	if err := os.WriteFile(registryPath, data, 0o644); err != nil {
		return fmt.Errorf("failed to write port registry: %w", err)
	}

	return nil
}

func initializeRegistryFromExistingApps(registry *PortRegistry, configDir string) error {
	appsDir := filepath.Join(configDir, "apps")
	entries, err := os.ReadDir(appsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // No apps directory yet, that's fine
		}
		return fmt.Errorf("failed to read apps directory: %w", err)
	}

	portPattern := regexp.MustCompile(`ports = \[ "127\.0\.0\.1:(\d+):\d+" \];`)

	for _, entry := range entries {
		if !strings.HasSuffix(entry.Name(), ".nix") {
			continue
		}

		appName := strings.TrimSuffix(entry.Name(), ".nix")
		filePath := filepath.Join(appsDir, entry.Name())

		content, err := os.ReadFile(filePath)
		if err != nil {
			continue // Skip files we can't read
		}

		matches := portPattern.FindSubmatch(content)
		if len(matches) >= 2 {
			if port, err := strconv.Atoi(string(matches[1])); err == nil {
				registry.Allocations[appName] = port
				if port >= registry.NextPort {
					registry.NextPort = port + 1
				}
			}
		}
	}

	return nil
}

func allocatePort(registry *PortRegistry, appName string) (int, error) {
	// Check if app already has a port allocated
	if port, exists := registry.Allocations[appName]; exists {
		return port, nil
	}

	// Find next available port
	for port := registry.NextPort; port <= PortRangeEnd; port++ {
		// Check if port is already allocated
		inUse := false
		for _, allocatedPort := range registry.Allocations {
			if allocatedPort == port {
				inUse = true
				break
			}
		}

		if !inUse {
			registry.Allocations[appName] = port
			registry.NextPort = port + 1
			return port, nil
		}
	}

	return 0, fmt.Errorf("no available ports in range %d-%d", PortRangeStart, PortRangeEnd)
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
		messages  []string
		mounts    []string
	)

	initCmd := &cobra.Command{
		Use:   "init",
		Short: "create a new nix config for an app",
		Run: func(cmd *cobra.Command, args []string) {
			// TUI only when no init flags are passed, or when only --dry-run is passed
			changedName := cmd.Flags().Changed("name")
			changedImage := cmd.Flags().Changed("image")
			changedDomain := cmd.Flags().Changed("domain")
			changedPort := cmd.Flags().Changed("port")
			changedSub := cmd.Flags().Changed("subdomain")
			changedNet := cmd.Flags().Changed("network")
			changedEnv := cmd.Flags().Changed("env-file")
			changedEdit := cmd.Flags().Changed("edit")
			changedMount := cmd.Flags().Changed("mount")
			changedDry := cmd.Flags().Changed("dry-run")

			anyInitFlag := changedName || changedImage || changedDomain || changedPort || changedSub || changedNet || changedEnv || changedEdit || changedMount || changedDry
			onlyDryRun := changedDry && !(changedName || changedImage || changedDomain || changedPort || changedSub || changedNet || changedEnv || changedEdit || changedMount)
			noInitFlags := !anyInitFlag

			usingTUI := onlyDryRun || noInitFlags

			if usingTUI {
				// Interactive: collect all required fields via TUI
				initial := AppConfig{
					ConfigDir: configDir,
					Network:   network,
					DryRun:    dryRun,
					EnvFile:   envFile,
					EditEnv:   edit,
					Mounts:    mounts,
				}
				cfg, ok, err := RunTUI(initial)
				if err != nil {
					fmt.Println(errorStyle.Render("Error: " + err.Error()))
					os.Exit(1)
				}
				if !ok {
					// user canceled
					return
				}
				generateAndWriteConfig(cfg)
				return
			}

			// Non-interactive: validate required flags
			missing := []string{}
			if name == "" {
				missing = append(missing, "--name")
			}
			if image == "" {
				missing = append(missing, "--image")
			}
			if domain == "" {
				missing = append(missing, "--domain")
			}
			if port <= 0 || port > 65535 {
				missing = append(missing, "--port (1-65535)")
			}
			if len(missing) > 0 {
				fmt.Println(errorStyle.Render("Missing required flags: ") + strings.Join(missing, ", "))
				fmt.Println(mutedStyle.Render("Either pass none of the required flags to use the interactive TUI, or provide all required flags."))
				os.Exit(1)
			}

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
				Mounts:    mounts,
			}
			generateAndWriteConfig(c)
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
	initCmd.Flags().StringArrayVar(&mounts, "mount", []string{}, "add a mount (e.g., /host:/container[:ro|rw] or name:/container[:ro|rw])")

	ghActionCmd := &cobra.Command{
		Use:   "gh-action",
		Short: "print GitHub Actions workflow for container deployment",
		Run: func(cmd *cobra.Command, args []string) {
			printGitHubAction(branch)
		},
	}
	ghActionCmd.Flags().StringVar(&branch, "branch", "main", "branch to deploy from")

	deployCmd := &cobra.Command{
		Use:   "deploy",
		Short: "commit and push changes to the rollouts repository",
		Run: func(cmd *cobra.Command, args []string) {
			runPushCommand(messages)
		},
	}
	deployCmd.Flags().StringArrayVarP(&messages, "message", "m", []string{}, "commit message (can be used multiple times for multi-line messages)")

	rootCmd.AddCommand(initCmd)
	rootCmd.AddCommand(ghActionCmd)
	rootCmd.AddCommand(deployCmd)
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func generateAndWriteConfig(app AppConfig) {
	// Load port registry
	registry, err := loadPortRegistry(app.ConfigDir)
	if err != nil {
		fmt.Println(errorStyle.Render("✗ Failed to load port registry: " + err.Error()))
		os.Exit(1)
	}

	// Allocate a port for this app
	hostPort, err := allocatePort(registry, app.Name)
	if err != nil {
		fmt.Println(errorStyle.Render("✗ Failed to allocate port: " + err.Error()))
		os.Exit(1)
	}

	config := NixAppConfig{
		Name:          app.Name,
		Image:         app.Image,
		Domain:        app.Domain,
		Subdomain:     app.Subdomain,
		ContainerPort: app.Port,
		Network:       app.Network,
		HasSecrets:    app.EnvFile != "" || (app.EditEnv && app.EnvFile == ""),
		HostPort:      hostPort,
		Mounts:        app.Mounts,
	}

	nixConfig := config.Generate()

	// Dry-run: print only raw config, no extra output
	if app.DryRun {
		fmt.Print(nixConfig)
		return
	}

	// Simplified summary (no boxes, no generated config echo)
	fmt.Println(headerStyle.Render("✨ Configuration Summary"))
	fmt.Printf("Name: %s\n", successStyle.Render(config.Name))
	fmt.Printf("Image: %s\n", successStyle.Render(config.Image))
	if config.Subdomain != "" {
		fmt.Printf("URL: %s\n", successStyle.Render(fmt.Sprintf("https://%s.%s", config.Subdomain, config.Domain)))
	} else {
		fmt.Printf("URL: %s\n", successStyle.Render(fmt.Sprintf("https://%s", config.Domain)))
	}
	fmt.Printf("Container Port: %s\n", successStyle.Render(fmt.Sprintf("%d", config.ContainerPort)))
	fmt.Printf("Host Port: %s\n", successStyle.Render(fmt.Sprintf("%d", config.HostPort)))
	fmt.Printf("Network: %s\n", successStyle.Render(config.Network))
	if config.HasSecrets {
		fmt.Printf("Secrets: %s\n", successStyle.Render("Enabled"))
	}
	if len(config.Mounts) > 0 {
		fmt.Printf("Mounts (%d):\n", len(config.Mounts))
		for _, mnt := range config.Mounts {
			fmt.Println("  - " + successStyle.Render(mnt))
		}
	}

	// File operations
	filePath := filepath.Join(app.ConfigDir, "apps", fmt.Sprintf("%s.nix", config.Name))
	err = os.WriteFile(filePath, []byte(nixConfig), 0o644)
	if err != nil {
		fmt.Println(errorStyle.Render("✗ Failed to write file: " + err.Error()))
		os.Exit(1)
	}
	fmt.Println(successStyle.Render("✓ Configuration written to " + filePath))

	// Save port registry after successful file write
	if err := savePortRegistry(registry, app.ConfigDir); err != nil {
		fmt.Println(errorStyle.Render("✗ Failed to save port registry: " + err.Error()))
		os.Exit(1)
	}

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

func runPushCommand(messages []string) {
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
	var commitMsg string
	if len(messages) > 0 {
		commitMsg = strings.Join(messages, "\n")
	} else {
		commitMsg = "rollout: automated commit via deploy command"
	}
	commitCmd := exec.Command("git", "commit", "-m", commitMsg)
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

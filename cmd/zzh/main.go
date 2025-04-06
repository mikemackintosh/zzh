package main

import (
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"os/user"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/kevinburke/ssh_config"
)

var (
	// Command line flags
	zzhMode    = flag.Bool("zzh", false, "Run in zzh panel mode")
	zzhPanelID = flag.String("panel-id", "", "zzh panel ID (when running in zzh mode)")
	zzhLogFile = flag.String("log-file", "", "Log file path (when running in zzh mode)")

	// Version information
	version = "0.1.0"
	commit  = "abc123"
	date    = "2023-10-01"
)

// SSHHost contains information about an SSH host from config
type SSHHost struct {
	name         string
	hostname     string
	user         string
	port         string
	identityFile string
}

// Implement the list.Item interface for SSHHost
func (s SSHHost) Title() string { return s.name }
func (s SSHHost) Description() string {
	return fmt.Sprintf("%s@%s:%s", s.user, s.hostname, s.port)
}
func (s SSHHost) FilterValue() string { return s.name }

// Model represents the application state
type model struct {
	list         list.Model
	selectedHost *SSHHost
	err          error
	spinner      spinner.Model
	connecting   bool
	width        int
	height       int
}

// Message types for tea.Cmd functions
type errorMsg struct {
	err error
}

// Initial command when starting the application
func (m model) Init() tea.Cmd {
	return nil
}

// Update function to handle messages and events
func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		// Handle key presses
		switch msg.String() {
		case "ctrl+c", "q":
			return m, tea.Quit

		case "enter":
			if m.list.SelectedItem() != nil && !m.connecting {
				host := m.list.SelectedItem().(SSHHost)
				m.selectedHost = &host

				// Set connecting flag
				m.connecting = true

				// Return selected host and tell the program to quit
				return m, tea.Quit
			}
		}

	case spinner.TickMsg:
		// Update spinner
		if m.connecting {
			var cmd tea.Cmd
			m.spinner, cmd = m.spinner.Update(msg)
			return m, cmd
		}

	case errorMsg:
		// Handle connection errors
		m.connecting = false
		m.err = msg.err
		return m, nil

	case tea.WindowSizeMsg:
		// Update window size
		m.width = msg.Width
		m.height = msg.Height

		// Update list dimensions
		top, right, bottom, left := listMargins()
		m.list.SetSize(
			msg.Width-left-right,
			msg.Height-top-bottom,
		)
	}

	// Handle list updates
	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	return m, cmd
}

// Render the view
func (m model) View() string {
	titleStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("39")).
		Background(lipgloss.Color("236")).
		Padding(0, 1).
		Bold(true)

	errorStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("196")).
		Bold(true)

	connectingStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("43")).
		Bold(true)

	helpStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("241")).
		Italic(true)

	if m.err != nil {
		return fmt.Sprintf("%s\n\n%s",
			errorStyle.Render("Error: "+m.err.Error()),
			helpStyle.Render("Press any key to quit."))
	}

	if m.connecting {
		return fmt.Sprintf("%s\n%s",
			connectingStyle.Render("Connecting to "+m.selectedHost.name+"..."),
			helpStyle.Render("Please wait."))
	}

	// Render the list of hosts
	return fmt.Sprintf(
		"%s\n\n%s\n\n%s",
		titleStyle.Render(fmt.Sprintf("SSH Hosts - %s", version)),
		m.list.View(),
		helpStyle.Render("Press q to quit, enter to connect."),
	)
}

// Define list styling
func listMargins() (top, right, bottom, left int) {
	return 1, 2, 1, 2
}

// Load SSH hosts from config file
func loadSSHHosts() ([]list.Item, error) {
	usr, err := user.Current()
	if err != nil {
		return nil, fmt.Errorf("failed to get current user: %w", err)
	}

	// Open SSH config file
	configFile := filepath.Join(usr.HomeDir, ".ssh", "config")
	f, err := os.Open(configFile)
	if err != nil {
		return nil, fmt.Errorf("failed to open SSH config file: %w", err)
	}
	defer f.Close()

	// Parse SSH config
	cfg, err := ssh_config.Decode(f)
	if err != nil {
		return nil, fmt.Errorf("failed to parse SSH config: %w", err)
	}

	// Extract hosts
	items := []list.Item{}
	for _, host := range cfg.Hosts {
		for _, pattern := range host.Patterns {
			// Skip wildcard/pattern hosts
			if strings.Contains(pattern.String(), "*") {
				continue
			}

			hostName := pattern.String()

			hostname := ssh_config.Get(hostName, "HostName")
			if hostname == "" {
				hostname = hostName
			}

			user := ssh_config.Get(hostName, "User")
			if user == "" {
				user = usr.Username
			}

			port := ssh_config.Get(hostName, "Port")
			if port == "" {
				port = "22"
			}

			// Get identity file
			identityFile := ssh_config.Get(hostName, "IdentityFile")
			if identityFile == "" {
				// Default to id_rsa if not specified
				identityFile = filepath.Join(usr.HomeDir, ".ssh", "id_rsa")
			} else if strings.HasPrefix(identityFile, "~") {
				// Expand ~ to home directory
				identityFile = strings.Replace(identityFile, "~", usr.HomeDir, 1)
			}

			items = append(items, SSHHost{
				name:         hostName,
				hostname:     hostname,
				user:         user,
				port:         port,
				identityFile: identityFile,
			})
		}
	}

	return items, nil
}

// Connect to SSH host using the native SSH client
func connectToSSHNative(host SSHHost, inZzhPanel bool) error {
	// Create a timestamp for the log file
	timestamp := time.Now().Format("20060102-150405")

	// Set default log file path if not running in zzh mode
	logFileName := fmt.Sprintf("ssh_session_%s_%s.log", host.name, timestamp)
	if *zzhLogFile != "" {
		logFileName = *zzhLogFile
	}

	logFile, err := os.Create(logFileName)
	if err != nil {
		return fmt.Errorf("failed to create log file: %w", err)
	}
	defer logFile.Close()

	// Log session start
	fmt.Fprintf(logFile, "=== SSH Session to %s started at %s ===\n\n",
		host.name, time.Now().Format(time.RFC3339))

	// Build the ssh command with arguments
	sshArgs := []string{}

	// Ensure terminal supports colors
	sshArgs = append(sshArgs, "-t")

	// Add identity file if specified
	if host.identityFile != "" {
		sshArgs = append(sshArgs, "-i", host.identityFile)
	}

	// Add port if not default
	if host.port != "22" {
		sshArgs = append(sshArgs, "-p", host.port)
	}

	// Add host address
	hostAddr := fmt.Sprintf("%s@%s", host.user, host.hostname)
	sshArgs = append(sshArgs, hostAddr)

	// Create the SSH command
	cmd := exec.Command("ssh", sshArgs...)

	// Set environment variables to ensure color support
	cmd.Env = append(os.Environ(), "TERM=xterm-256color")

	// Set up I/O
	cmd.Stdin = os.Stdin
	cmd.Stdout = io.MultiWriter(os.Stdout, logFile)
	cmd.Stderr = io.MultiWriter(os.Stderr, logFile)

	// Start the SSH command
	err = cmd.Start()
	if err != nil {
		return fmt.Errorf("failed to start SSH: %w", err)
	}

	// Set up signal handling
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)

	// Wait for command completion or signal
	go func() {
		for sig := range sigChan {
			// Forward signals to SSH process
			if cmd.Process != nil {
				cmd.Process.Signal(sig)
			}
		}
	}()

	// Wait for SSH to complete
	err = cmd.Wait()

	// Stop signal handling
	signal.Stop(sigChan)
	close(sigChan)

	// Log session end
	fmt.Fprintf(logFile, "=== SSH Session ended at %s ===\n",
		time.Now().Format(time.RFC3339))

	if err != nil && cmd.ProcessState.ExitCode() != 0 {
		return fmt.Errorf("SSH exited with code %d: %w",
			cmd.ProcessState.ExitCode(), err)
	}

	return nil
}

// Connect to SSH host via zzh panel command
func connectToSSHViaZzh(host SSHHost) error {
	// Create a timestamp for the log file
	timestamp := time.Now().Format("20060102-150405")
	logFileName := fmt.Sprintf("ssh_session_%s_%s.log", host.name, timestamp)
	if *zzhLogFile != "" {
		logFileName = *zzhLogFile
	}

	logFile, err := os.Create(logFileName)
	if err != nil {
		return fmt.Errorf("failed to create log file: %w", err)
	}
	defer logFile.Close()

	// Log session start
	fmt.Fprintf(logFile, "=== SSH Session to %s via zzh panel started at %s ===\n\n",
		host.name, time.Now().Format(time.RFC3339))

	// Build the zzh command with arguments to connect to the host
	zzhArgs := []string{"connect"}

	// Add host name
	zzhArgs = append(zzhArgs, host.name)

	// If panel ID is specified, add it
	if *zzhPanelID != "" {
		zzhArgs = append(zzhArgs, "--panel-id", *zzhPanelID)
	}

	// Create the zzh command
	cmd := exec.Command("zzh", zzhArgs...)

	// Set environment variables
	cmd.Env = append(os.Environ(), "TERM=xterm-256color")

	// Set up I/O
	cmd.Stdin = os.Stdin
	cmd.Stdout = io.MultiWriter(os.Stdout, logFile)
	cmd.Stderr = io.MultiWriter(os.Stderr, logFile)

	// Start the zzh command
	err = cmd.Start()
	if err != nil {
		return fmt.Errorf("failed to start zzh connect: %w", err)
	}

	// Set up signal handling
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)

	// Wait for command completion or signal
	go func() {
		for sig := range sigChan {
			// Forward signals to zzh process
			if cmd.Process != nil {
				cmd.Process.Signal(sig)
			}
		}
	}()

	// Wait for zzh to complete
	err = cmd.Wait()

	// Stop signal handling
	signal.Stop(sigChan)
	close(sigChan)

	// Log session end
	fmt.Fprintf(logFile, "=== SSH Session ended at %s ===\n",
		time.Now().Format(time.RFC3339))

	if err != nil && cmd.ProcessState.ExitCode() != 0 {
		return fmt.Errorf("zzh exited with code %d: %w",
			cmd.ProcessState.ExitCode(), err)
	}

	return nil
}

func setupLogging() (*os.File, error) {
	var logPath string
	if *zzhLogFile != "" {
		// Use the specified log file path
		logPath = *zzhLogFile + ".app.log"
	} else {
		logPath = "ssh_selector.log"
	}

	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
	if err != nil {
		return nil, fmt.Errorf("failed to open log file: %w", err)
	}

	log.SetOutput(logFile)
	return logFile, nil
}

func main() {
	// Parse command line flags
	flag.Parse()

	// Set up logging
	logFile, err := setupLogging()
	if err != nil {
		fmt.Printf("Failed to set up logging: %v\n", err)
		os.Exit(1)
	}
	defer logFile.Close()

	// Load SSH hosts
	items, err := loadSSHHosts()
	if err != nil {
		fmt.Printf("Error loading SSH hosts: %v\n", err)
		os.Exit(1)
	}

	// Create delegate for list items
	delegate := list.NewDefaultDelegate()
	delegate.ShowDescription = true

	// Enhanced styling with more colors
	delegate.Styles.SelectedTitle = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder(), false, false, false, true).
		BorderForeground(lipgloss.Color("170")).
		Foreground(lipgloss.Color("170")).
		Bold(true)

	delegate.Styles.SelectedDesc = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder(), false, false, false, true).
		BorderForeground(lipgloss.Color("170")).
		Foreground(lipgloss.Color("240"))

	delegate.Styles.NormalTitle = lipgloss.NewStyle().
		Foreground(lipgloss.Color("39")).
		Bold(true)

	delegate.Styles.NormalDesc = lipgloss.NewStyle().
		Foreground(lipgloss.Color("246"))

	// Set up list with sensible defaults
	l := list.New(items, delegate, 80, 20)
	l.Title = fmt.Sprintf("SSH Hosts - %s", version)
	l.SetShowStatusBar(true)
	l.SetFilteringEnabled(true)
	l.Styles.Title = lipgloss.NewStyle().
		Foreground(lipgloss.Color("39")).
		Background(lipgloss.Color("236")).
		Padding(0, 1).
		MarginBottom(1).
		Bold(true)
	l.Styles.PaginationStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	l.Styles.HelpStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))

	// Set up spinner
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("170"))

	// Create our model
	m := model{
		list:    l,
		spinner: s,
	}

	// Create and start a new Bubbletea program
	p := tea.NewProgram(m, tea.WithAltScreen())
	result, err := p.Run()
	if err != nil {
		fmt.Printf("Error running program: %v\n", err)
		os.Exit(1)
	}

	// Check if a host was selected
	if selectedResult, ok := result.(model); ok {
		if selectedResult.selectedHost != nil {
			// UI has quit but we have a selected host - connect to it
			host := *selectedResult.selectedHost

			// Determine whether to use native SSH or zzh panel integration
			var connectErr error
			if *zzhMode {
				// Use zzh to connect
				connectErr = connectToSSHViaZzh(host)
			} else {
				// Use native SSH
				connectErr = connectToSSHNative(host, false)
			}

			if connectErr != nil {
				fmt.Printf("Error: %v\n", connectErr)
				os.Exit(1)
			}

			// After SSH session is done
			fmt.Println("SSH session completed.")
		}
	}
}

package git

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// Repository represents a git repository
type Repository struct {
	Path string
}

// NewRepository creates a new git repository instance
func NewRepository(path string) *Repository {
	return &Repository{
		Path: path,
	}
}

// Status represents the status of files in the repository
type Status struct {
	Modified  []string `json:"modified"`
	Added     []string `json:"added"`
	Deleted   []string `json:"deleted"`
	Untracked []string `json:"untracked"`
	Clean     bool     `json:"clean"`
}

// CommitInfo represents information about a git commit
type CommitInfo struct {
	Hash      string    `json:"hash"`
	Author    string    `json:"author"`
	Date      time.Time `json:"date"`
	Message   string    `json:"message"`
	ShortHash string    `json:"short_hash"`
}

// IsGitRepository checks if the path is a git repository
func (r *Repository) IsGitRepository() bool {
	gitDir := filepath.Join(r.Path, ".git")
	if stat, err := os.Stat(gitDir); err == nil {
		return stat.IsDir()
	}
	return false
}

// IsGitAvailable checks if git command is available
func IsGitAvailable() bool {
	_, err := exec.LookPath("git")
	return err == nil
}

// Status returns the current status of the repository
func (r *Repository) Status() (*Status, error) {
	if !r.IsGitRepository() {
		return nil, fmt.Errorf("not a git repository: %s", r.Path)
	}

	cmd := exec.Command("git", "status", "--porcelain")
	cmd.Dir = r.Path
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to get git status: %w", err)
	}

	status := &Status{
		Modified:  []string{},
		Added:     []string{},
		Deleted:   []string{},
		Untracked: []string{},
		Clean:     true,
	}

	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	for _, line := range lines {
		if line == "" {
			continue
		}

		status.Clean = false
		if len(line) < 3 {
			continue
		}

		statusCode := line[:2]
		fileName := line[3:]

		switch statusCode {
		case "M ", " M", "MM":
			status.Modified = append(status.Modified, fileName)
		case "A ", " A", "AM":
			status.Added = append(status.Added, fileName)
		case "D ", " D", "AD", "MD":
			status.Deleted = append(status.Deleted, fileName)
		case "??":
			status.Untracked = append(status.Untracked, fileName)
		default:
			// Handle other cases as modified
			status.Modified = append(status.Modified, fileName)
		}
	}

	return status, nil
}

// Add stages files for commit
func (r *Repository) Add(files ...string) error {
	if !r.IsGitRepository() {
		return fmt.Errorf("not a git repository: %s", r.Path)
	}

	if len(files) == 0 {
		files = []string{"."}
	}

	args := append([]string{"add"}, files...)
	cmd := exec.Command("git", args...)
	cmd.Dir = r.Path
	
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to add files: %w", err)
	}

	return nil
}

// Commit creates a new commit with the given message
func (r *Repository) Commit(message string) error {
	if !r.IsGitRepository() {
		return fmt.Errorf("not a git repository: %s", r.Path)
	}

	if message == "" {
		return fmt.Errorf("commit message cannot be empty")
	}

	cmd := exec.Command("git", "commit", "-m", message)
	cmd.Dir = r.Path
	
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to commit: %w", err)
	}

	return nil
}

// AddAndCommit stages all changes and commits them with the given message
func (r *Repository) AddAndCommit(message string) error {
	if err := r.Add(); err != nil {
		return fmt.Errorf("failed to add files: %w", err)
	}

	if err := r.Commit(message); err != nil {
		return fmt.Errorf("failed to commit: %w", err)
	}

	return nil
}

// Log returns the commit history
func (r *Repository) Log(limit int) ([]CommitInfo, error) {
	if !r.IsGitRepository() {
		return nil, fmt.Errorf("not a git repository: %s", r.Path)
	}

	args := []string{"log", "--pretty=format:%H|%an|%ad|%s", "--date=iso"}
	if limit > 0 {
		args = append(args, fmt.Sprintf("-%d", limit))
	}

	cmd := exec.Command("git", args...)
	cmd.Dir = r.Path
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to get git log: %w", err)
	}

	var commits []CommitInfo
	scanner := bufio.NewScanner(strings.NewReader(string(output)))
	
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}

		parts := strings.Split(line, "|")
		if len(parts) != 4 {
			continue
		}

		date, err := time.Parse("2006-01-02 15:04:05 -0700", parts[2])
		if err != nil {
			// Try alternative format
			date, _ = time.Parse("2006-01-02 15:04:05", parts[2])
		}

		commit := CommitInfo{
			Hash:      parts[0],
			Author:    parts[1],
			Date:      date,
			Message:   parts[3],
			ShortHash: parts[0][:8],
		}

		commits = append(commits, commit)
	}

	return commits, nil
}

// HasChanges checks if there are any uncommitted changes
func (r *Repository) HasChanges() (bool, error) {
	status, err := r.Status()
	if err != nil {
		return false, err
	}
	return !status.Clean, nil
}

// GetLastCommit returns information about the last commit
func (r *Repository) GetLastCommit() (*CommitInfo, error) {
	commits, err := r.Log(1)
	if err != nil {
		return nil, err
	}
	
	if len(commits) == 0 {
		return nil, fmt.Errorf("no commits found")
	}
	
	return &commits[0], nil
}

// ValidateRepository ensures the repository is in a valid state for operations
func (r *Repository) ValidateRepository() error {
	if !IsGitAvailable() {
		return fmt.Errorf("git command not available")
	}

	if !r.IsGitRepository() {
		return fmt.Errorf("not a git repository: %s", r.Path)
	}

	// Check if directory exists and is accessible
	if _, err := os.Stat(r.Path); err != nil {
		return fmt.Errorf("repository path not accessible: %w", err)
	}

	return nil
}

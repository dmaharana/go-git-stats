package main

import (
	"encoding/csv"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

// RepoBranchCommits holds commit data aggregated by date for a specific repo and branch.
type RepoBranchCommits struct {
	RepoName      string
	BranchName    string
	CommitsByDate map[string]int // map[DateString]CommitCount
}

type SummaryEntry struct {
	Date  string
	Count int
}

func main() {
	zerolog.TimeFieldFormat = zerolog.TimeFormatUnix
	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr})

	// Define command line flags
	baseDir := flag.String("dir", ".", "Base directory to scan for Git repositories")
	batchSize := flag.Int("batch", 5, "Number of repositories to process concurrently")
	flag.Parse()

	// Find all git repositories
	repos, err := findGitRepos(*baseDir)
	if err != nil {
		log.Error().Err(err).Msg("Error finding git repositories")
		os.Exit(1)
	}

	log.Info().Int("repo_count", len(repos)).Msg("Found git repositories")

	// Process repositories in batches
	results := processReposBatch(repos, *batchSize)

	// Ensure baseDir is absolute for reliable output path generation
	outputDir, err := filepath.Abs(*baseDir)
	if err != nil {
		log.Error().Err(err).Msg("Error resolving absolute path for base directory")
		os.Exit(1)
	}

	// Write results to CSV files in the specified base directory
	if err := writeCommitInfoCSV(results, outputDir); err != nil {
		log.Error().Err(err).Msg("Error writing commit info CSV")
		os.Exit(1)
	}

	if err := writeSummaryCSV(results, outputDir); err != nil {
		log.Error().Err(err).Msg("Error writing summary CSV")
		os.Exit(1)
	}
	if err := writeYearlySummaryCSV(results, outputDir); err != nil {
		log.Error().Err(err).Msg("Error writing yearly summary CSV")
		os.Exit(1)
	}

	log.Info().Str("output_dir", outputDir).Msg("Scan completed successfully. CSV files generated")
}

// findGitRepos finds all bare git repositories in the given directory
func findGitRepos(baseDir string) ([]string, error) {
	var repos []string

	err := filepath.WalkDir(baseDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		// Check if directory is a bare git repository (has config and HEAD files)
		if d.IsDir() {
			configPath := filepath.Join(path, "config")
			headPath := filepath.Join(path, "HEAD")
			_, configErr := os.Stat(configPath)
			_, headErr := os.Stat(headPath)
			if configErr == nil && headErr == nil && strings.HasSuffix(path, ".git") {
				repos = append(repos, path)
			}
		}
		return nil
	})

	return repos, err
}

// processReposBatch processes repositories in concurrent batches
func processReposBatch(repoPaths []string, batchSize int) []RepoBranchCommits {
	var results []RepoBranchCommits
	var mutex sync.Mutex
	var wg sync.WaitGroup

	// Process repositories in batches
	for i := 0; i < len(repoPaths); i += batchSize {
		end := i + batchSize
		if end > len(repoPaths) {
			end = len(repoPaths)
		}

		batch := repoPaths[i:end]
		wg.Add(len(batch))

		for _, repoPath := range batch {
			go func(path string) {
				defer wg.Done()
				repoInfo, err := processRepo(path)
				if err != nil {
					log.Error().Err(err).Str("repo_path", path).Msg("Error processing repository")
					return
				}

				mutex.Lock()
				results = append(results, repoInfo...)
				mutex.Unlock()
			}(repoPath)
		}

		wg.Wait()
	}

	return results
}

// processRepo processes a single git repository
func processRepo(repoPath string) ([]RepoBranchCommits, error) {
	repo, err := git.PlainOpen(repoPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open repository: %w", err)
	}

	// Get repository name
	repoName := filepath.Base(filepath.Dir(repoPath))
	if filepath.Base(repoPath) == ".git" {
		repoName = filepath.Base(filepath.Dir(repoPath))
	} else {
		repoName = filepath.Base(repoPath)
	}

	// Get all branches
	branches, err := repo.References()
	if err != nil {
		return nil, fmt.Errorf("failed to get references: %w", err)
	}

	var results []RepoBranchCommits

	err = branches.ForEach(func(ref *plumbing.Reference) error {
		// Only process local branches (refs/heads/)
		if !ref.Name().IsBranch() {
			return nil
		}

		branchName := ref.Name().Short()
		commitsByDate := make(map[string]int) // map[DateString]CommitCount

		// Get commit history for this branch
		commitIter, err := repo.Log(&git.LogOptions{From: ref.Hash()})
		if err != nil {
			// Handle case where a branch might not have a resolvable commit (e.g., empty branch)
			// Or other errors during Log retrieval
			log.Warn().Err(err).Str("branch", branchName).Str("repo", repoName).Msg("Could not get commit history")
			return nil // Skip this branch
		}

		func() {
			defer func() {
				if r := recover(); r != nil {
					log.Warn().Interface("panic", r).Str("branch", branchName).Str("repo", repoName).Msg("Recovered from panic during commit iteration")
				}
			}()

			iterErr := commitIter.ForEach(func(c *object.Commit) error {
				dateStr := c.Author.When.Format("2006-01-02")
				commitsByDate[dateStr]++
				return nil
			})

			if iterErr != nil {
				log.Warn().Err(iterErr).Str("branch", branchName).Str("repo", repoName).Msg("Error during commit iteration")
			}
		}()

		// Add partial or full results if any commits were counted
		if len(commitsByDate) > 0 {
			results = append(results, RepoBranchCommits{
				RepoName:      repoName,
				BranchName:    branchName,
				CommitsByDate: commitsByDate,
			})
		}

		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("failed to process branches: %w", err)
	}

	return results, nil
}

// writeCommitInfoCSV writes the detailed commit info per date/repo/branch to a CSV file
func writeCommitInfoCSV(results []RepoBranchCommits, outputDir string) error {
	timestamp := time.Now().Format("20060102150405")
	filename := fmt.Sprintf("commit_info_%s.csv", timestamp)
	filepath := filepath.Join(outputDir, filename)

	file, err := os.Create(filepath)
	if err != nil {
		return fmt.Errorf("failed to create %s: %w", filename, err)
	}
	defer file.Close()

	writer := csv.NewWriter(file)
	defer writer.Flush()

	// Write header - TASK.md: date,repo,branch,commit_count
	header := []string{"Date", "Repository", "Branch", "CommitCount"}
	if err := writer.Write(header); err != nil {
		return fmt.Errorf("failed to write header to %s: %w", filename, err)
	}

	// Write data
	for _, result := range results {
		// Sort dates for consistent output order
		var dates []string
		for date := range result.CommitsByDate {
			dates = append(dates, date)
		}
		sort.Strings(dates)

		for _, date := range dates {
			count := result.CommitsByDate[date]
			row := []string{
				date,
				result.RepoName,
				result.BranchName,
				strconv.Itoa(count),
			}
			if err := writer.Write(row); err != nil {
				return fmt.Errorf("failed to write row to %s: %w", filename, err)
			}
		}
	}

	return nil
}

// writeYearlySummaryCSV writes the yearly commit summary to a CSV file
func writeYearlySummaryCSV(results []RepoBranchCommits, outputDir string) error {
	timestamp := time.Now().Format("20060102150405")
	filename := fmt.Sprintf("yearly_summary_%s.csv", timestamp)
	filepath := filepath.Join(outputDir, filename)

	file, err := os.Create(filepath)
	if err != nil {
		return fmt.Errorf("failed to create %s: %w", filename, err)
	}
	defer file.Close()

	writer := csv.NewWriter(file)
	defer writer.Flush()

	// Write header - TASK.md: year,commit_count
	header := []string{"Year", "CommitCount"}
	if err := writer.Write(header); err != nil {
		return fmt.Errorf("failed to write header to %s: %w", filename, err)
	}

	// Aggregate commits by year from all repos/branches
	commitsByYear := make(map[int]int) // map[Year]TotalCommitCount
	for _, result := range results {
		for dateStr, count := range result.CommitsByDate {
			// Parse date string to get the year
			commitDate, err := time.Parse("2006-01-02", dateStr)
			if err != nil {
				// Should not happen if date format is correct
				log.Warn().Err(err).Str("date", dateStr).Str("repo", result.RepoName).Str("branch", result.BranchName).Msg("Could not parse date")
				continue
			}
			commitsByYear[commitDate.Year()] += count
		}
	}

	// Convert to slice for sorting
	type YearlySummary struct {
		Year  int
		Count int
	}
	var summaries []YearlySummary
	for year, count := range commitsByYear {
		summaries = append(summaries, YearlySummary{
			Year:  year,
			Count: count,
		})
	}

	// Sort by year
	sort.Slice(summaries, func(i, j int) bool {
		return summaries[i].Year < summaries[j].Year
	})

	// Write data
	for _, summary := range summaries {
		row := []string{
			strconv.Itoa(summary.Year),
			strconv.Itoa(summary.Count),
		}
		if err := writer.Write(row); err != nil {
			return fmt.Errorf("failed to write row to %s: %w", filename, err)
		}
	}

	return nil
}

// writeSummaryCSV writes the daily commit summary across all repos/branches to a CSV file
func writeSummaryCSV(results []RepoBranchCommits, outputDir string) error {
	timestamp := time.Now().Format("20060102150405")
	filename := fmt.Sprintf("commit_summary_%s.csv", timestamp)
	filepath := filepath.Join(outputDir, filename)

	file, err := os.Create(filepath)
	if err != nil {
		return fmt.Errorf("failed to create %s: %w", filename, err)
	}
	defer file.Close()

	writer := csv.NewWriter(file)
	defer writer.Flush()

	// Write header - TASK.md: date,commit_count
	header := []string{"Date", "CommitCount"}
	if err := writer.Write(header); err != nil {
		return fmt.Errorf("failed to write header to %s: %w", filename, err)
	}

	// Aggregate commits by date across all repos/branches
	commitsByDate := make(map[string]int) // map[DateString]TotalCommitCount
	for _, result := range results {
		for date, count := range result.CommitsByDate {
			commitsByDate[date] += count
		}
	}

	// Convert to slice for sorting
	var summaries []SummaryEntry
	for date, count := range commitsByDate {
		summaries = append(summaries, SummaryEntry{
			Date:  date,
			Count: count,
		})
	}

	// Sort by date
	sort.Slice(summaries, func(i, j int) bool {
		return summaries[i].Date < summaries[j].Date
	})

	// Write data
	for _, summary := range summaries {
		row := []string{
			summary.Date,
			strconv.Itoa(summary.Count),
		}
		if err := writer.Write(row); err != nil {
			return fmt.Errorf("failed to write row to %s: %w", filename, err)
		}
	}

	return nil
}

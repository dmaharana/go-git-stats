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

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
)

type CommitInfo struct {
	Year          int
	RepoName      string
	BranchName    string
	CommitCount   int
	CommitsByDate map[string]int
}

type SummaryEntry struct {
	Date  string
	Count int
}

func main() {
	// Define command line flags
	baseDir := flag.String("dir", ".", "Base directory to scan for Git repositories")
	batchSize := flag.Int("batch", 5, "Number of repositories to process concurrently")
	flag.Parse()

	// Find all git repositories
	repos, err := findGitRepos(*baseDir)
	if err != nil {
		fmt.Printf("Error finding git repositories: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Found %d git repositories\n", len(repos))
	// Process repositories in batches
	results := processReposBatch(repos, *batchSize)

	// Write results to CSV files
	if err := writeCommitInfoCSV(results); err != nil {
		fmt.Printf("Error writing commit info CSV: %v\n", err)
		os.Exit(1)
	}

	if err := writeSummaryCSV(results); err != nil {
		fmt.Printf("Error writing summary CSV: %v\n", err)
		os.Exit(1)
	}
	if err := writeYearlySummaryCSV(results); err != nil {
		fmt.Printf("Error writing yearly summary CSV: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("Scan completed successfully. CSV files generated: commit_info.csv, commit_summary.csv, yearly_summary.csv")
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
func processReposBatch(repoPaths []string, batchSize int) []CommitInfo {
	var results []CommitInfo
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
					fmt.Printf("Error processing repository %s: %v\n", path, err)
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
func processRepo(repoPath string) ([]CommitInfo, error) {
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

	var results []CommitInfo

	err = branches.ForEach(func(ref *plumbing.Reference) error {
		if ref.Type() != plumbing.HashReference || ref.Name().IsBranch() == false {
			return nil
		}

		branchName := ref.Name().Short()
		commitsByYear := make(map[int]int)
		commitsByDate := make(map[string]int)

		// Get commit history for this branch
		commitIter, err := repo.Log(&git.LogOptions{From: ref.Hash()})
		if err != nil {
			return fmt.Errorf("failed to get commit history for branch %s: %w",
				branchName, err)
		}

		err = commitIter.ForEach(func(c *object.Commit) error {
			year := c.Author.When.Year()
			dateStr := c.Author.When.Format("2006-01-02")
			commitsByYear[year]++
			commitsByDate[dateStr]++
			return nil
		})

		if err != nil {
			return fmt.Errorf("failed to iterate commits: %w", err)
		}

		// Create CommitInfo entries for each year
		for year, count := range commitsByYear {
			results = append(results, CommitInfo{
				Year:          year,
				RepoName:      repoName,
				BranchName:    branchName,
				CommitCount:   count,
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

// writeCommitInfoCSV writes the commit info to a CSV file
func writeCommitInfoCSV(results []CommitInfo) error {
	file, err := os.Create("commit_info.csv")
	if err != nil {
		return fmt.Errorf("failed to create commit_info.csv: %w", err)
	}
	defer file.Close()

	writer := csv.NewWriter(file)
	defer writer.Flush()

	// Write header
	header := []string{"Year", "Repository", "Branch", "CommitCount"}
	if err := writer.Write(header); err != nil {
		return fmt.Errorf("failed to write header: %w", err)
	}

	// Write data
	for _, info := range results {
		row := []string{
			strconv.Itoa(info.Year),
			info.RepoName,
			info.BranchName,
			strconv.Itoa(info.CommitCount),
		}
		if err := writer.Write(row); err != nil {
			return fmt.Errorf("failed to write row: %w", err)
		}
	}

	return nil
}

// writeYearlySummaryCSV writes the yearly commit summary to a CSV file
func writeYearlySummaryCSV(results []CommitInfo) error {
	file, err := os.Create("yearly_summary.csv")
	if err != nil {
		return fmt.Errorf("failed to create yearly_summary.csv: %w", err)
	}
	defer file.Close()

	writer := csv.NewWriter(file)
	defer writer.Flush()

	// Write header
	header := []string{"Year", "CommitCount"}
	if err := writer.Write(header); err != nil {
		return fmt.Errorf("failed to write header: %w", err)
	}

	// Combine all commits by year
	commitsByYear := make(map[int]int)
	for _, info := range results {
		commitsByYear[info.Year] += info.CommitCount
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
			return fmt.Errorf("failed to write row: %w", err)
		}
	}

	return nil
}

// writeSummaryCSV writes the summary info to a CSV file
func writeSummaryCSV(results []CommitInfo) error {
	file, err := os.Create("commit_summary.csv")
	if err != nil {
		return fmt.Errorf("failed to create commit_summary.csv: %w", err)
	}
	defer file.Close()

	writer := csv.NewWriter(file)
	defer writer.Flush()

	// Write header
	header := []string{"Date", "CommitCount"}
	if err := writer.Write(header); err != nil {
		return fmt.Errorf("failed to write header: %w", err)
	}

	// Combine all commit dates
	commitsByDate := make(map[string]int)
	for _, info := range results {
		for date, count := range info.CommitsByDate {
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
			return fmt.Errorf("failed to write row: %w", err)
		}
	}

	return nil
}

# gitstat

`gitstat` is a command-line tool written in Go that analyzes Git repositories to generate commit statistics. It scans a directory for Git repositories, processes their commit history, and outputs the results to CSV files.

## Features

- Scans a base directory to find Git repositories (including bare repositories).
- Processes commit history for each branch in each repository.
- Generates three CSV files:
  - `commit_info.csv`: Detailed commit information per year, repository, and branch.
  - `commit_summary.csv`: Summary of commit counts per date across all repositories.
  - `yearly_summary.csv`: Summary of commit counts per year across all repositories.
- Supports concurrent processing of repositories to speed up analysis.

## Prerequisites

- Go programming language (version 1.16 or later)
- Git

## Building

To build the `gitstat` tool, you can use the provided `Makefile`.

**For Linux:**

1. Run `make linux` from the source directory.
2. The resulting binary will be in the `bin` directory.

**For Windows:**

1. Run `make windows` from the source directory.
2. The resulting binary will be in the `bin` directory.

## Usage

The `gitstat` tool takes a single argument: the path to the base directory containing the Git repositories to be analyzed. The tool will scan the directory recursively to find Git repositories (including bare repositories).

Example:

`gitstat --dir /path/to/base/directory --batch 10`

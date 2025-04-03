1. Get the git bare repos from the base folder
2. Get the commit information from each repository, like date and commit count from all branches
3. Concurrently process the repositories, but the branches should be processed only one at a time
4. Create 3 CSV files with below headers
   - date,repo,branch,commit_count
   - date,commit_count
   - year,commit_count

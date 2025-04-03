1. Use English as default language
2. This will be a go command line tool
3. The tool will be called gitstat
4. This tool will read the git bare repos from the base folder
5. This tool will read the commit information from each repo
6. This tool will write the commit information to the CSV files
7. The CSV files will be created in the base folder
8. The CSV files will be named as below, with timestamp in the file name
   - commit_info.csv
   - commit_summary.csv
   - commit_year_summary.csv
9. Setup a standard go project structure
10. Ensure to process the bare repos in parallel, but the branches should be processed only one at a time

package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"it-interview-question-bank-tools/tools/internal/bank"
)

func main() {
	repo := flag.String("repo", ".", "path to the question-bank repository")
	write := flag.Bool("write", false, "apply the validated changes; without this flag the command only checks")
	reindex := flag.Bool("reindex", false, "renumber all question IDs in source order and update answer IDs")
	flag.Parse()

	absRepo, err := filepath.Abs(*repo)
	if err != nil {
		fatal(err)
	}

	mode := "check"
	if *write {
		mode = "write"
	}
	if *reindex {
		mode = "reindex-check"
		if *write {
			mode = "reindex-write"
		}
	}
	fmt.Printf("Mode: %s\nRepository: %s\n", mode, absRepo)

	languages := []bank.Language{bank.RU, bank.EN}
	if *write {
		for _, language := range languages {
			var normalizeErr error
			if *reindex {
				_, normalizeErr = bank.Reindex(absRepo, language, false)
			} else {
				_, normalizeErr = bank.Normalize(absRepo, language, false)
			}
			if normalizeErr != nil {
				fatal(fmt.Errorf("pre-check %s: %w", language, normalizeErr))
			}
		}
		fmt.Println("Pre-check for RU and EN passed.")
	}

	for _, language := range languages {
		var report bank.NormalizationReport
		var normalizeErr error
		if *reindex {
			report, normalizeErr = bank.Reindex(absRepo, language, *write)
		} else {
			report, normalizeErr = bank.Normalize(absRepo, language, *write)
		}
		if normalizeErr != nil {
			fatal(fmt.Errorf("%s: %w", language, normalizeErr))
		}
		fmt.Printf("%s: questions=%d, reindexed_ids=%d, new_question_ids=%d, answers=%d, answer_headings_updated=%d, quote_lines_removed=%d, changed_files=%d\n",
			report.Language,
			report.Questions,
			report.ReindexedIDs,
			report.NewQuestionIDs,
			report.Answers,
			report.RepairedHeadings,
			report.RemovedQuoteLines,
			report.ChangedFiles,
		)
	}

	if !*write {
		fmt.Println("No files were changed. Run again with --write after reviewing the checks.")
	} else {
		fmt.Println("Validated changes were applied atomically per file.")
	}
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "ERROR:", err)
	os.Exit(1)
}

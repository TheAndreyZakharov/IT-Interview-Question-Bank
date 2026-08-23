package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"it-interview-question-bank-tools/tools/internal/bank"
)

func main() {
	repo := flag.String("repo", ".", "path to the question-bank repository")
	limit := flag.Int("limit", 100, "maximum number of unanswered questions to print per language; 0 means all")
	flag.Parse()
	if *limit < 0 {
		fatal(fmt.Errorf("limit must be zero or positive"))
	}

	absRepo, err := filepath.Abs(*repo)
	if err != nil {
		fatal(err)
	}
	fmt.Printf("Repository: %s\n", absRepo)

	languages := []bank.Language{bank.RU, bank.EN}
	statsByLanguage := make([]bank.Stats, 0, len(languages))
	for _, language := range languages {
		stats, collectErr := bank.CollectStats(absRepo, language)
		if collectErr != nil {
			fatal(fmt.Errorf("%s: %w", language, collectErr))
		}
		statsByLanguage = append(statsByLanguage, stats)
	}

	fmt.Println("\n=== Общая статистика банка ===")
	for _, stats := range statsByLanguage {
		printOverallStats(stats)
	}

	fmt.Println("\n=== Уже рассмотренные разделы: вопросы без ответа ===")
	for _, stats := range statsByLanguage {
		printReviewedStats(stats, *limit)
	}
}

func printOverallStats(stats bank.Stats) {
	coverage := 0.0
	if stats.Questions > 0 {
		coverage = float64(stats.AnsweredQuestions) * 100 / float64(stats.Questions)
	}
	fmt.Printf("%s: вопросов %d, отвечено %d, без ответа %d, покрытие %.2f%%\n",
		stats.Language,
		stats.Questions,
		stats.AnsweredQuestions,
		len(stats.Unanswered),
		coverage,
	)
	if stats.AnswerRootMissing {
		fmt.Println("   Ответные файлы: папка отсутствует")
	}
	fmt.Println()
}

func printReviewedStats(stats bank.Stats, limit int) {
	if stats.AnswerRootMissing || len(stats.ReviewedSections) == 0 {
		fmt.Printf("%s: рассмотренных разделов нет — пропускаю поиск.\n", stats.Language)
		return
	}

	progress := 0.0
	if stats.ReviewedQuestions > 0 {
		progress = float64(stats.ReviewedAnswered) * 100 / float64(stats.ReviewedQuestions)
	}
	fmt.Printf("\n%s: разделов %d, вопросов в них %d, отвечено %d, без ответа %d, прогресс %.2f%%\n",
		stats.Language,
		len(stats.ReviewedSections),
		stats.ReviewedQuestions,
		stats.ReviewedAnswered,
		len(stats.ReviewedUnanswered),
		progress,
	)
	fmt.Printf("   Рассмотренные разделы: %s\n", strings.Join(stats.ReviewedSections, ", "))

	printIssues(stats)
	if len(stats.ReviewedUnanswered) == 0 {
		fmt.Println("   Все вопросы в уже рассмотренных разделах имеют ответы.")
		return
	}

	shown := len(stats.ReviewedUnanswered)
	if limit > 0 && shown > limit {
		shown = limit
	}
	fmt.Printf("   Вопросы без ответа (%d из %d):\n", shown, len(stats.ReviewedUnanswered))
	for _, question := range stats.ReviewedUnanswered[:shown] {
		id := question.ID
		if id == "" {
			id = "NO-ID"
		}
		fmt.Printf("   - [%s] [раздел %s] %s\n", id, question.Section, question.Text)
	}
	if shown < len(stats.ReviewedUnanswered) {
		fmt.Println("   ... для полного списка используй --limit 0")
	}
}

func printIssues(stats bank.Stats) {
	if len(stats.UnmatchedAnswers) > 0 {
		fmt.Printf("   Проблемы сопоставления ответов: %d\n", len(stats.UnmatchedAnswers))
		for _, answer := range stats.UnmatchedAnswers {
			fmt.Printf("   - %s\n", answer.Text)
		}
	}
	if len(stats.DuplicateAnswerID) > 0 {
		fmt.Printf("   Дубликаты ID ответов: %d\n", len(stats.DuplicateAnswerID))
		for _, duplicate := range stats.DuplicateAnswerID {
			fmt.Printf("   - %s\n", duplicate)
		}
	}
	if len(stats.MismatchedAnswerID) > 0 {
		fmt.Printf("   Несовпадения ID ответов: %d\n", len(stats.MismatchedAnswerID))
		for _, mismatch := range stats.MismatchedAnswerID {
			fmt.Printf("   - %s\n", mismatch)
		}
	}
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "ERROR:", strings.TrimSpace(err.Error()))
	os.Exit(1)
}

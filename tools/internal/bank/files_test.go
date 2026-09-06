package bank

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestProcessAnswerFileRefreshesPlainOutlineID(t *testing.T) {
	path := filepath.Join(t.TempDir(), "### 5.1 Frontend.md")
	content := strings.Join([]string{
		"## 5. Frontend",
		"",
		"### 5.1 Frontend basics",
		"",
		"- What is frontend development? [id: RU-000001]",
		"",
	}, "\n")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	question := &Question{ID: "RU-000123", Text: "What is frontend development?", Section: "5.1"}
	index := &Index{
		Language:      RU,
		BySectionText: map[string][]*Question{sectionKey(question.Section, question.Text): {question}},
		ByText:        map[string][]*Question{canonicalText(question.Text): {question}},
	}
	changes, staged, answers, _, err := processAnswerFile(path, RU, index, true)
	if err != nil {
		t.Fatalf("processAnswerFile() error = %v", err)
	}
	defer cleanupStaged([]StagedFile{staged})
	if len(answers) != 0 {
		t.Fatalf("answers = %d, want 0 for an outline", len(answers))
	}
	if len(changes) != 1 || changes[0].Kind != "outline-id" {
		t.Fatalf("changes = %#v, want one outline-id change", changes)
	}
	updated, err := os.ReadFile(staged.Temp)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(updated), "- What is frontend development? [id: RU-000123]") {
		t.Fatalf("staged outline was not updated:\n%s", updated)
	}
}

func TestProcessAnswerFilePreservesBlockquotes(t *testing.T) {
	path := filepath.Join(t.TempDir(), "### 5.1 Frontend.md")
	content := strings.Join([]string{
		"## 5. Frontend",
		"",
		"### 5.1 Frontend basics",
		"",
		"- **What is frontend development?** [id: RU-000001]",
		"Ответ:",
		"> This quotation is part of the answer and must remain a blockquote.",
		"> - This quoted list item must not become a question outline.",
		"",
	}, "\n")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	question := &Question{ID: "RU-000123", Text: "What is frontend development?", Section: "5.1"}
	index := &Index{
		Language:      RU,
		BySectionText: map[string][]*Question{sectionKey(question.Section, question.Text): {question}},
		ByText:        map[string][]*Question{canonicalText(question.Text): {question}},
	}
	_, staged, answers, quoteLinesRemoved, err := processAnswerFile(path, RU, index, true)
	if err != nil {
		t.Fatalf("processAnswerFile() error = %v", err)
	}
	defer cleanupStaged([]StagedFile{staged})
	if len(answers) != 1 {
		t.Fatalf("answers = %d, want 1", len(answers))
	}
	if quoteLinesRemoved != 0 {
		t.Fatalf("quoteLinesRemoved = %d, want 0", quoteLinesRemoved)
	}
	updated, err := os.ReadFile(staged.Temp)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(updated), "> This quotation is part of the answer and must remain a blockquote.") ||
		!strings.Contains(string(updated), "> - This quoted list item must not become a question outline.") {
		t.Fatalf("blockquotes were changed:\n%s", updated)
	}
}

func TestParseSourceQuestionRequiresPlainListMarker(t *testing.T) {
	text, id, prefix, ok, err := parseSourceQuestion("- What is frontend development? [id: RU-000123]", RU)
	if err != nil || !ok {
		t.Fatalf("plain question parsed as ok=%t, err=%v", ok, err)
	}
	if text != "What is frontend development?" || id != "RU-000123" || prefix != "" {
		t.Fatalf("plain question = text=%q id=%q prefix=%q", text, id, prefix)
	}
	_, _, _, quotedOK, err := parseSourceQuestion("> - What is frontend development? [id: RU-000123]", RU)
	if err != nil || quotedOK {
		t.Fatalf("quoted list item parsed as ok=%t, err=%v", quotedOK, err)
	}
}

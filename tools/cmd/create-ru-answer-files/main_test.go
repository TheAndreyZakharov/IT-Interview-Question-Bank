package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPlansForSourceCreatesOnlyTerminalSectionsInRange(t *testing.T) {
	sections := []*section{
		{code: "3", level: 2, heading: "## 3. База"},
		{code: "3.2", level: 3, heading: "### 3.2 Старый раздел", questions: []string{"- Старый вопрос? [id: RU-000001]"}},
		{code: "3.7", level: 3, heading: "### 3.7 Безопасность"},
		{code: "3.7.1", level: 4, heading: "#### 3.7.1 OAuth2 / OIDC: основы", questions: []string{"- Что такое OAuth2? [id: RU-000002]"}},
		{code: "3.7.2", level: 4, heading: "#### 3.7.2 JWT", questions: []string{"- Что такое JWT? [id: RU-000003]"}},
	}

	plans, err := plansForSource(sections, "/answer-root", mustParseCode("3.7"), mustParseCode("20.12"))
	if err != nil {
		t.Fatalf("plansForSource() error = %v", err)
	}
	if len(plans) != 2 {
		t.Fatalf("plansForSource() produced %d files, want 2", len(plans))
	}
	if got, want := plans[0].path, "/answer-root/## 3. База/### 3.7 Безопасность/#### 3.7.1 OAuth2 - OIDC- основы.md"; got != want {
		t.Fatalf("first path = %q, want %q", got, want)
	}
	if strings.Contains(plans[0].content, "3.2 Старый раздел") {
		t.Fatal("older section was included in generated content")
	}
	if !strings.Contains(plans[0].content, "## 3. База\n\n### 3.7 Безопасность\n\n#### 3.7.1 OAuth2 / OIDC: основы") {
		t.Fatalf("generated hierarchy is incorrect:\n%s", plans[0].content)
	}
}

func TestReadSectionsSkipsCompletedPartOfTopicThree(t *testing.T) {
	path := filepath.Join(t.TempDir(), "## 3. База.md")
	content := strings.Join([]string{
		"## 3. База",
		"",
		"### 3.6 Уже готовый раздел",
		"- Старый вопрос без ID не должен разбираться генератором",
		"",
		"### 3.7 Новый раздел",
		"",
		"#### 3.7.1 Новый конечный раздел",
		"- Новый вопрос? [id: RU-000010]",
		"",
	}, "\n")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	sections, err := readSections(path, 3, mustParseCode("3.7"))
	if err != nil {
		t.Fatalf("readSections() error = %v", err)
	}
	if len(sections) != 3 {
		t.Fatalf("readSections() retained %d sections, want 3", len(sections))
	}
	if got, want := []string{sections[0].code, sections[1].code, sections[2].code}, []string{"3", "3.7", "3.7.1"}; strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("retained sections = %v, want %v", got, want)
	}
	if len(sections[2].questions) != 1 || sections[2].questions[0] != "- Новый вопрос? [id: RU-000010]" {
		t.Fatalf("new section questions = %v", sections[2].questions)
	}
}

func TestWritePlansNeverOverwritesExistingFile(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "## 4. Языки", "### 4.1 Go.md")
	plan := filePlan{section: "4.1", path: path, content: "new content\n"}

	if err := writePlans([]filePlan{plan}); err != nil {
		t.Fatalf("first writePlans() error = %v", err)
	}
	if err := ensureTargetsAreNew([]filePlan{plan}); err == nil {
		t.Fatal("ensureTargetsAreNew() allowed an existing target")
	}
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "new content\n" {
		t.Fatalf("existing content changed to %q", content)
	}
}

func TestCompareCodes(t *testing.T) {
	if compareCodes(mustParseCode("3.7.1"), mustParseCode("3.7")) <= 0 {
		t.Fatal("3.7.1 must be after 3.7")
	}
	if compareCodes(mustParseCode("20.12"), mustParseCode("20.12")) != 0 {
		t.Fatal("equal codes must compare equally")
	}
	if compareCodes(mustParseCode("20.12"), mustParseCode("20.13")) >= 0 {
		t.Fatal("20.12 must be before 20.13")
	}
}

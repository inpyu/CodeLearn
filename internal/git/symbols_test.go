package git_test

import (
	"testing"

	"github.com/inpyu/codelearn/internal/git"
)

func outlineNames(path, text string) []string {
	var out []string
	for _, symbol := range git.Outline(path, text) {
		out = append(out, symbol.Kind+":"+symbol.Name)
	}
	return out
}

func equal(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestGoOutline(t *testing.T) {
	source := "package m\n\nimport \"os\"\n\ntype Thing struct {\n\tA int\n}\n\n" +
		"func (t *Thing) Do() int {\n\treturn 1\n}\n\nfunc Run() int {\n\treturn 2\n}\n"

	got := outlineNames("m.go", source)
	want := []string{"type:Thing", "method:Thing.Do", "function:Run"}
	if !equal(got, want) {
		t.Fatalf("got %v want %v", got, want)
	}
}

func TestPythonOutline(t *testing.T) {
	source := "import os\n\n\ndef top():\n    return 1\n\n\nclass Thing:\n    size = 1\n\n    def method(self):\n        return 2\n"

	got := outlineNames("m.py", source)
	want := []string{"function:top", "class:Thing", "method:Thing.method"}
	if !equal(got, want) {
		t.Fatalf("got %v want %v", got, want)
	}
}

func TestPythonClassOwnsOnlyLinesAboveItsFirstMethod(t *testing.T) {
	source := "class Thing:\n    size = 1\n\n    def method(self):\n        return 2\n"

	thing := git.Outline("m.py", source)[0]
	if thing.Start != 1 || thing.End != 3 {
		t.Fatalf("class range = %d-%d, want 1-3", thing.Start, thing.End)
	}
}

func TestPythonDecoratorsBelongToTheFunction(t *testing.T) {
	source := "import x\n\n\n@decorator\ndef wrapped():\n    return 1\n"

	if got := git.Outline("m.py", source)[0].Start; got != 4 {
		t.Fatalf("start = %d, want 4", got)
	}
}

func TestNestedHelperStaysInsideItsFunction(t *testing.T) {
	source := "def outer():\n    def inner():\n        return 1\n\n    return inner\n"

	if got := outlineNames("m.py", source); !equal(got, []string{"function:outer"}) {
		t.Fatalf("got %v", got)
	}
}

func TestCommentAboveADefinitionBelongsToIt(t *testing.T) {
	source := "package m\n\n// Explains Run.\nfunc Run() int {\n\treturn 1\n}\n"

	if got := git.Outline("m.go", source)[0].Start; got != 3 {
		t.Fatalf("start = %d, want 3", got)
	}
}

func TestTypeScriptOutline(t *testing.T) {
	source := "import x from 'x'\n\nexport interface Props {\n  a: string\n}\n\n" +
		"export const helper = (value: string) => {\n  return value\n}\n\n" +
		"export default function Component({\n  a,\n}: Props) {\n  return a\n}\n"

	got := outlineNames("c.tsx", source)
	want := []string{"type:Props", "function:helper", "function:Component"}
	if !equal(got, want) {
		t.Fatalf("got %v want %v", got, want)
	}
}

func TestDestructuredParameterListDoesNotEndTheFunction(t *testing.T) {
	source := "export default function C({\n  a,\n  b,\n}: Props) {\n  return a\n}\n"

	component := git.Outline("c.tsx", source)[0]
	if component.Start != 1 || component.End != 6 {
		t.Fatalf("range = %d-%d, want 1-6", component.Start, component.End)
	}
}

func TestClassMethodsInABraceLanguage(t *testing.T) {
	source := "class Box {\n  size = 1\n\n  open() {\n    return 1\n  }\n}\n"

	got := outlineNames("b.ts", source)
	if !equal(got, []string{"class:Box", "method:Box.open"}) {
		t.Fatalf("got %v", got)
	}
}

func TestUnsupportedLanguagesHaveNoOutline(t *testing.T) {
	if got := git.Outline("notes.md", "# Title\n\ntext\n"); got != nil {
		t.Fatalf("markdown produced %v", got)
	}
	if got := git.Outline("data.json", "{\"a\": 1}\n"); got != nil {
		t.Fatalf("json produced %v", got)
	}
	if got := git.Outline("m.go", ""); got != nil {
		t.Fatalf("empty file produced %v", got)
	}
}

func TestLabelFormatsCallablesWithParentheses(t *testing.T) {
	source := "package m\n\ntype Thing struct{}\n\nfunc Run() int {\n\treturn 1\n}\n"

	var labels []string
	for _, symbol := range git.Outline("m.go", source) {
		labels = append(labels, symbol.Label())
	}
	if !equal(labels, []string{"Thing", "Run()"}) {
		t.Fatalf("labels = %v", labels)
	}
}

func TestPythonMultiLineSignatureKeepsItsBody(t *testing.T) {
	source := "def wide(\n    a,\n    b,\n):\n    return a\n\n\ndef after():\n    return 2\n"

	symbols := git.Outline("m.py", source)
	if len(symbols) != 2 {
		t.Fatalf("symbols = %v", outlineNames("m.py", source))
	}
	// The closing "):" is indented no further than the def, so a naive
	// indentation scan would end the block there and drop the body.
	if symbols[0].Start != 1 || symbols[0].End != 5 {
		t.Fatalf("wide = %d-%d, want 1-5", symbols[0].Start, symbols[0].End)
	}
}

func TestPythonMethodWithAMultiLineSignature(t *testing.T) {
	source := "class C:\n    def m(\n        self,\n        a,\n    ):\n        return a\n"

	symbols := git.Outline("m.py", source)
	if len(symbols) != 2 || symbols[1].Name != "C.m" {
		t.Fatalf("symbols = %v", outlineNames("m.py", source))
	}
	if symbols[1].Start != 2 || symbols[1].End != 6 {
		t.Fatalf("C.m = %d-%d, want 2-6", symbols[1].Start, symbols[1].End)
	}
}

func TestGenericArrowFunctionIsFound(t *testing.T) {
	source := "export const useThing = <T,>(\n  initial: T,\n  { eager = false }: { eager?: boolean } = {},\n) => {\n  return initial\n}\n"

	symbols := git.Outline("x.ts", source)
	if len(symbols) != 1 || symbols[0].Name != "useThing" {
		t.Fatalf("symbols = %v", outlineNames("x.ts", source))
	}
	if symbols[0].Start != 1 || symbols[0].End != 6 {
		t.Fatalf("useThing = %d-%d, want 1-6", symbols[0].Start, symbols[0].End)
	}
}

func TestMultiLineTypeAliasIsNotClipped(t *testing.T) {
	source := "export type Status =\n  | 'a'\n  | 'b'\n\nexport function After() {\n  return 1\n}\n"

	symbols := git.Outline("x.ts", source)
	if len(symbols) != 2 {
		t.Fatalf("symbols = %v", outlineNames("x.ts", source))
	}
	if symbols[0].Start != 1 || symbols[0].End != 3 {
		t.Fatalf("Status = %d-%d, want 1-3", symbols[0].Start, symbols[0].End)
	}
}

func TestGoMultiLineSignatureAndGenerics(t *testing.T) {
	source := "package m\n\nfunc Collect(\n\tctx context.Context,\n\trepo *Repository,\n) (*Snapshot, error) {\n\treturn nil, nil\n}\n\nfunc Map[T any, U any](in []T) []U {\n\treturn nil\n}\n"

	symbols := git.Outline("x.go", source)
	if len(symbols) != 2 {
		t.Fatalf("symbols = %v", outlineNames("x.go", source))
	}
	if symbols[0].Start != 3 || symbols[0].End != 8 {
		t.Fatalf("Collect = %d-%d, want 3-8", symbols[0].Start, symbols[0].End)
	}
	if symbols[1].Name != "Map" {
		t.Fatalf("second symbol = %q", symbols[1].Name)
	}
}

func TestBlockCommentBracesDoNotConfuseTheScanner(t *testing.T) {
	source := "/* a { brace */\nexport function First() {\n  return 1\n}\n\nexport function Second() {\n  return 2\n}\n"

	if got := outlineNames("x.js", source); !equal(got, []string{"function:First", "function:Second"}) {
		t.Fatalf("got %v", got)
	}
}

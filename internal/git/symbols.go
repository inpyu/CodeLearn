package git

import (
	"regexp"
	"strings"
)

// Symbol kinds.
const (
	KindFunction = "function"
	KindMethod   = "method"
	KindClass    = "class"
	KindType     = "type"
	KindBlock    = "block"
)

// Symbol is a named region of a file, in inclusive 1-based line numbers.
//
// Start and End delimit the symbol's own lines: a class that contains methods
// owns only the lines above its first method, and the methods are symbols of
// their own. That keeps every unit contiguous.
type Symbol struct {
	Name  string
	Kind  string
	Start int
	End   int
}

// Label is how the unit is named in the viewer.
func (s Symbol) Label() string {
	if s.Kind == KindFunction || s.Kind == KindMethod {
		return s.Name + "()"
	}
	return s.Name
}

// Outline returns the symbols of a file, or nil when the language is not
// supported. Outlines are best effort and dependency free: a file that cannot
// be outlined simply falls back to raw hunks as its change units.
func Outline(path, text string) []Symbol {
	if text == "" {
		return nil
	}
	lines := strings.Split(text, "\n")
	switch extensionOf(path) {
	case "py":
		return attachComments(pythonOutline(lines), lines, "#")
	case "go":
		return attachComments(braceOutline(lines, goPatterns, false), lines, "//")
	case "js", "jsx", "mjs", "cjs", "ts", "tsx":
		return attachComments(braceOutline(lines, jsPatterns, true), lines, "//")
	case "c", "cc", "cpp", "cxx", "h", "hpp", "java", "cs", "rs", "swift", "kt":
		return attachComments(braceOutline(lines, cPatterns, true), lines, "//")
	}
	return nil
}

func extensionOf(path string) string {
	name := path
	if index := strings.LastIndex(path, "/"); index >= 0 {
		name = path[index+1:]
	}
	dot := strings.LastIndex(name, ".")
	if dot < 0 {
		return ""
	}
	return strings.ToLower(name[dot+1:])
}

// -- Python -----------------------------------------------------------------

var (
	pyDef   = regexp.MustCompile(`^(\s*)(?:async\s+)?def\s+([A-Za-z_]\w*)`)
	pyClass = regexp.MustCompile(`^(\s*)class\s+([A-Za-z_]\w*)`)
)

// pythonOutline scans by indentation rather than parsing. Only classes are
// descended into: a helper nested in a function stays part of that function,
// which keeps a function whole.
func pythonOutline(lines []string) []Symbol {
	var symbols []Symbol
	for index := 0; index < len(lines); index++ {
		indent, name, kind, ok := pythonHeader(lines[index])
		if !ok || indent != 0 {
			continue
		}
		start, end := index+1, pythonBlockEnd(lines, index, indent)
		if kind == KindClass {
			methods := pythonMethods(lines, index+1, end, name)
			own := end
			if len(methods) > 0 {
				own = methods[0].Start - 1
			}
			symbols = append(symbols, Symbol{name, KindClass, withDecorators(lines, start), max(start, own)})
			symbols = append(symbols, methods...)
			continue
		}
		symbols = append(symbols, Symbol{name, KindFunction, withDecorators(lines, start), end})
		index = end - 1
	}
	return symbols
}

func pythonHeader(line string) (indent int, name, kind string, ok bool) {
	if m := pyClass.FindStringSubmatch(line); m != nil {
		return len(m[1]), m[2], KindClass, true
	}
	if m := pyDef.FindStringSubmatch(line); m != nil {
		return len(m[1]), m[2], KindFunction, true
	}
	return 0, "", "", false
}

// pythonBlockEnd is the last line of the block opened at index.
//
// The scan starts after the signature rather than after the header line: a
// def whose parameters span several lines ends its signature on a line
// indented no further than the def itself ("):"), and treating that as the
// end of the block would leave the body outside the unit.
func pythonBlockEnd(lines []string, index, indent int) int {
	index = pythonSignatureEnd(lines, index)
	end := index + 1
	for next := index + 1; next < len(lines); next++ {
		line := lines[next]
		if strings.TrimSpace(line) == "" {
			continue
		}
		if leadingSpaces(line) <= indent {
			return end
		}
		end = next + 1
	}
	return end
}

// pythonSignatureEnd is the index of the line closing the parameter list.
func pythonSignatureEnd(lines []string, index int) int {
	depth := 0
	for at := index; at < len(lines); at++ {
		for _, char := range lines[at] {
			switch char {
			case '(', '[', '{':
				depth++
			case ')', ']', '}':
				depth--
			case '#':
				goto nextLine
			}
		}
	nextLine:
		if depth <= 0 {
			return at
		}
	}
	return index
}

func pythonMethods(lines []string, first, last int, owner string) []Symbol {
	var methods []Symbol
	for index := first; index < last; index++ {
		indent, name, kind, ok := pythonHeader(lines[index])
		if !ok || indent == 0 || kind == KindClass {
			continue
		}
		end := pythonBlockEnd(lines, index, indent)
		if end > last {
			end = last
		}
		methods = append(methods, Symbol{
			owner + "." + name, KindMethod, withDecorators(lines, index+1), end,
		})
		index = end - 1
	}
	return methods
}

// withDecorators walks back over the decorator lines above a definition, which
// are part of the definition rather than module level code.
func withDecorators(lines []string, start int) int {
	for start > 1 {
		previous := strings.TrimSpace(lines[start-2])
		if !strings.HasPrefix(previous, "@") {
			break
		}
		start--
	}
	return start
}

func leadingSpaces(line string) int {
	return len(line) - len(strings.TrimLeft(line, " \t"))
}

// -- Brace languages --------------------------------------------------------

type pattern struct {
	re   *regexp.Regexp
	kind string
}

var goPatterns = []pattern{
	{regexp.MustCompile(`^func\s+\(\s*\w+\s+\*?(\w+)\s*\)\s*(\w+)`), KindMethod},
	{regexp.MustCompile(`^func\s+(\w+)`), KindFunction},
	{regexp.MustCompile(`^type\s+(\w+)`), KindType},
}

var jsPatterns = []pattern{
	{regexp.MustCompile(`^(?:export\s+)?(?:default\s+)?(?:async\s+)?function\s*\*?\s*([A-Za-z_$][\w$]*)`), KindFunction},
	{regexp.MustCompile(`^(?:export\s+)?(?:const|let|var)\s+([A-Za-z_$][\w$]*)\s*(?::[^=]+?)?=\s*(?:async\s+)?(?:function\b|<[^=]*>\s*\(|\(|[A-Za-z_$][\w$]*\s*=>)`), KindFunction},
	{regexp.MustCompile(`^(?:export\s+)?(?:abstract\s+)?class\s+([A-Za-z_$][\w$]*)`), KindClass},
	{regexp.MustCompile(`^(?:export\s+)?(?:declare\s+)?(?:interface|type|enum)\s+([A-Za-z_$][\w$]*)`), KindType},
}

var cPatterns = []pattern{
	{regexp.MustCompile(`^(?:public\s+|private\s+|protected\s+|static\s+|final\s+|abstract\s+)*(?:class|struct|interface|enum)\s+([A-Za-z_]\w*)`), KindClass},
	{regexp.MustCompile(`^(?:pub\s+)?(?:async\s+)?fn\s+([A-Za-z_]\w*)`), KindFunction},
	{regexp.MustCompile(`^(?:[A-Za-z_][\w:<>,*&\s\[\]]*?\s[*&]?)?([A-Za-z_][\w:~]*)\s*\([^;]*\)(?:\s*(?:const|noexcept|override))*\s*\{`), KindFunction},
}

var methodPattern = regexp.MustCompile(
	`^\s+(?:public\s+|private\s+|protected\s+|static\s+|async\s+|get\s+|set\s+|readonly\s+)*([A-Za-z_$][\w$]*)\s*(?:<[^>]*>)?\s*\([^;{]*\)\s*(?::[^{;]+)?\{`)

var methodKeywords = map[string]bool{"if": true, "for": true, "while": true, "switch": true, "catch": true}

func braceOutline(lines []string, patterns []pattern, withMethods bool) []Symbol {
	var symbols []Symbol
	for index := 0; index < len(lines); index++ {
		line := lines[index]
		if strings.TrimSpace(line) == "" || leadingSpaces(line) > 0 {
			continue
		}
		name, kind, ok := matchPatterns(line, patterns)
		if !ok {
			continue
		}
		endIndex := blockEnd(lines, index)
		start, end := index+1, endIndex+1

		var methods []Symbol
		if withMethods && kind == KindClass {
			methods = classMethods(lines, index+1, endIndex, name)
		}
		own := end
		if len(methods) > 0 {
			own = methods[0].Start - 1
		}
		symbols = append(symbols, Symbol{name, kind, start, max(start, own)})
		symbols = append(symbols, methods...)
		index = endIndex
	}
	return symbols
}

func matchPatterns(line string, patterns []pattern) (string, string, bool) {
	for _, p := range patterns {
		m := p.re.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		if p.kind == KindMethod && len(m) > 2 {
			return m[1] + "." + m[2], KindMethod, true
		}
		return m[1], p.kind, true
	}
	return "", "", false
}

// clean strips string literals and line comments so brace counting is reliable.
func clean(line string) string {
	var out strings.Builder
	var quote byte
	for index := 0; index < len(line); index++ {
		char := line[index]
		if quote != 0 {
			if char == '\\' {
				index++
				continue
			}
			if char == quote {
				quote = 0
			}
			continue
		}
		if char == '"' || char == '\'' || char == '`' {
			quote = char
			continue
		}
		if char == '/' && index+1 < len(line) && (line[index+1] == '/' || line[index+1] == '*') {
			break
		}
		out.WriteByte(char)
	}
	return out.String()
}

// blockEnd finds the line closing the block that opens at startIndex. Brace
// depth is only counted outside parentheses, so a destructured parameter list
// spread over several lines is not mistaken for the body.
func blockEnd(lines []string, startIndex int) int {
	depth, paren := 0, 0
	opened := false
	lastContinued := startIndex
	for index := startIndex; index < len(lines); index++ {
		text := clean(lines[index])
		for _, char := range text {
			switch char {
			case '(':
				paren++
			case ')':
				if paren > 0 {
					paren--
				}
			case '{':
				if paren == 0 {
					depth++
					opened = true
				}
			case '}':
				if paren == 0 {
					depth--
					if opened && depth <= 0 {
						return index
					}
				}
			}
		}
		if !opened && paren == 0 && strings.HasSuffix(strings.TrimRight(text, " \t"), ";") {
			return index
		}
		if !opened && paren == 0 && index > startIndex {
			// A declaration with no body - a type alias, say - continues for
			// as long as its lines are indented or open with a continuation
			// token, so a union spread over several lines stays one symbol
			// instead of being clipped to its first line.
			if !isContinuation(lines[index]) {
				return lastContinued
			}
			lastContinued = index
		}
	}
	if !opened {
		return lastContinued
	}
	return len(lines) - 1
}

// isContinuation reports whether a line carries on the declaration above it.
func isContinuation(line string) bool {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" {
		return false
	}
	if leadingSpaces(line) > 0 {
		return true
	}
	switch trimmed[0] {
	case '|', '&', ',', '.', '?', ':', '+':
		return true
	}
	return false
}

func classMethods(lines []string, first, last int, owner string) []Symbol {
	var methods []Symbol
	for index := first; index < last; index++ {
		m := methodPattern.FindStringSubmatch(lines[index])
		if m == nil || methodKeywords[m[1]] {
			continue
		}
		end := blockEnd(lines, index)
		if end > last {
			end = last
		}
		methods = append(methods, Symbol{owner + "." + m[1], KindMethod, index + 1, end + 1})
		index = end
	}
	return methods
}

// attachComments pulls the comment block above a definition into it: a section
// comment explains the symbol that follows, so it belongs to that unit.
func attachComments(symbols []Symbol, lines []string, prefix string) []Symbol {
	for i := range symbols {
		floor := 1
		for _, earlier := range symbols[:i] {
			if earlier.End+1 > floor {
				floor = earlier.End + 1
			}
		}
		start := symbols[i].Start
		for start > floor {
			text := ""
			if start-2 < len(lines) {
				text = strings.TrimSpace(lines[start-2])
			}
			if text != "" && !strings.HasPrefix(text, prefix) {
				break
			}
			start--
		}
		// Trim the blank lines the walk passed over, so the unit begins at the
		// comment itself rather than at the gap above it.
		for start < symbols[i].Start && strings.TrimSpace(lines[start-1]) == "" {
			start++
		}
		if start < symbols[i].Start && strings.HasPrefix(strings.TrimSpace(lines[start-1]), prefix) {
			symbols[i].Start = start
		}
	}
	return symbols
}

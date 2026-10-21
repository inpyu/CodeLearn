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
	blank := func(index int) bool {
		return index < 0 || index >= len(lines) || strings.TrimSpace(lines[index]) == ""
	}

	if extensionOf(path) == "py" {
		hash := func(index int) bool {
			return index >= 0 && index < len(lines) &&
				strings.HasPrefix(strings.TrimSpace(lines[index]), "#")
		}
		return attachComments(pythonOutline(lines), hash, blank)
	}

	patterns, methods, ok := bracePatterns(extensionOf(path))
	if !ok {
		return nil
	}
	cleaned, commentOnly := scanComments(lines)
	comment := func(index int) bool {
		return index >= 0 && index < len(commentOnly) && commentOnly[index]
	}
	return attachComments(braceOutline(cleaned, patterns, methods), comment, blank)
}

// bracePatterns selects the declaration patterns for a brace language, and
// reports whether class members are worth scanning for in it.
func bracePatterns(extension string) (patterns []pattern, methods, ok bool) {
	switch extension {
	case "go":
		return goPatterns, false, true
	case "js", "jsx", "mjs", "cjs", "ts", "tsx":
		return jsPatterns, true, true
	case "c", "cc", "cpp", "cxx", "h", "hpp", "java", "cs", "rs", "swift", "kt":
		return cPatterns, true, true
	}
	return nil, false, false
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

// braceOutline works from code with comments and strings already removed, so
// a declaration that only appears inside a comment or a string is not matched.
func braceOutline(cleaned []string, patterns []pattern, withMethods bool) []Symbol {
	var symbols []Symbol
	for index := 0; index < len(cleaned); index++ {
		line := cleaned[index]
		if strings.TrimSpace(line) == "" || leadingSpaces(line) > 0 {
			continue
		}
		name, kind, ok := matchPatterns(line, patterns)
		if !ok {
			continue
		}
		endIndex := blockEnd(cleaned, index)
		start, end := index+1, endIndex+1

		var methods []Symbol
		if withMethods && kind == KindClass {
			methods = classMethods(cleaned, index+1, endIndex, name)
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

// scanComments returns each line with its comments and string literals
// removed, so brace counting and pattern matching only ever see code.
//
// The scan carries state between lines, which is what the previous per-line
// cleaner could not do: a brace inside a block comment that spans several
// lines used to end a function early, and a multi-line raw string containing
// code used to invent a symbol that does not exist.
func scanComments(lines []string) (cleaned []string, commentOnly []bool) {
	cleaned = make([]string, len(lines))
	commentOnly = make([]bool, len(lines))
	inComment := false
	inRaw := false

	for index, line := range lines {
		var out strings.Builder
		var quote byte
		sawComment, sawString := false, false

		for at := 0; at < len(line); at++ {
			char := line[at]

			switch {
			case inComment:
				sawComment = true
				if char == '*' && at+1 < len(line) && line[at+1] == '/' {
					inComment = false
					at++
				}
			case inRaw:
				sawString = true
				// A raw string ends only at its closing backtick; it has no
				// escapes, so the character before it is irrelevant.
				if char == '`' {
					inRaw = false
				}
			case quote != 0:
				sawString = true
				if char == '\\' {
					at++
				} else if char == quote {
					quote = 0
				}
			case char == '`':
				inRaw = true
				sawString = true
			case char == '"' || char == '\'':
				quote = char
				sawString = true
			case char == '/' && at+1 < len(line) && line[at+1] == '/':
				sawComment = true
				at = len(line) // the rest of the line is a comment
			case char == '/' && at+1 < len(line) && line[at+1] == '*':
				inComment = true
				sawComment = true
				at++
			default:
				out.WriteByte(char)
			}
		}

		// A quoted string does not continue onto the next line in these
		// languages; a block comment and a raw string do.
		quote = 0
		cleaned[index] = out.String()
		// Why a line ended up empty matters: a comment above a definition
		// belongs to it, but the inside of a multi-line string does not.
		commentOnly[index] = sawComment && !sawString &&
			strings.TrimSpace(out.String()) == "" && strings.TrimSpace(line) != ""
	}
	return cleaned, commentOnly
}

// blockEnd finds the line closing the block that opens at startIndex. Brace
// depth is only counted outside parentheses, so a destructured parameter list
// spread over several lines is not mistaken for the body.
func blockEnd(cleaned []string, startIndex int) int {
	depth, paren := 0, 0
	opened := false
	lastContinued := startIndex
	for index := startIndex; index < len(cleaned); index++ {
		text := cleaned[index]
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
			if !isContinuation(cleaned[index]) {
				return lastContinued
			}
			lastContinued = index
		}
	}
	if !opened {
		return lastContinued
	}
	return len(cleaned) - 1
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

func classMethods(cleaned []string, first, last int, owner string) []Symbol {
	var methods []Symbol
	for index := first; index < last; index++ {
		m := methodPattern.FindStringSubmatch(cleaned[index])
		if m == nil || methodKeywords[m[1]] {
			continue
		}
		end := blockEnd(cleaned, index)
		if end > last {
			end = last
		}
		methods = append(methods, Symbol{owner + "." + m[1], KindMethod, index + 1, end + 1})
		index = end
	}
	return methods
}

// attachComments pulls the comment block above a definition into it: a doc
// or section comment explains the symbol that follows, so it belongs to that
// unit rather than floating as a module level fragment.
//
// isComment decides what a comment line is, which differs by language: a
// brace language has "//" and "/* ... */" blocks, Python has "#".
func attachComments(symbols []Symbol, isComment func(index int) bool, blank func(index int) bool) []Symbol {
	for i := range symbols {
		floor := 1
		for _, earlier := range symbols[:i] {
			if earlier.End+1 > floor {
				floor = earlier.End + 1
			}
		}
		start := symbols[i].Start
		for start > floor && (isComment(start-2) || blank(start-2)) {
			start--
		}
		// Trim the blank lines the walk passed over, so the unit begins at the
		// comment itself rather than at the gap above it.
		for start < symbols[i].Start && blank(start-1) {
			start++
		}
		if start < symbols[i].Start && isComment(start-1) {
			symbols[i].Start = start
		}
	}
	return symbols
}

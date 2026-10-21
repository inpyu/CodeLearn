package git

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
)

// changeID is the SHA-256 of a normalised serialisation of what identifies a
// unit: where it lives, what it is called, and the code on both sides.
//
// Line numbers are deliberately not part of it. They would make an id depend
// on everything above the unit in its file, so adding one import would
// invalidate every explanation below it even though none of that code
// changed. Leaving them out means an id survives exactly as long as its own
// code does, which is the property import relies on.
//
// occurrence disambiguates units that are otherwise identical - two module
// level blocks with the same content in one file - and is only ever non-zero
// for those, so it cannot reintroduce the positional dependency.
func changeID(baseCommit, path, symbol, before, after string, occurrence int) string {
	digest := sha256.New()
	for _, part := range []string{
		baseCommit, path, symbol, before, after, fmt.Sprintf("%d", occurrence),
	} {
		digest.Write([]byte(part))
		digest.Write([]byte{0})
	}
	return "sha256:" + hex.EncodeToString(digest.Sum(nil))
}

// occurrenceCounter hands out the index of each repeated unit identity.
type occurrenceCounter map[string]int

func (c occurrenceCounter) next(symbol, before, after string) int {
	key := symbol + "\x00" + before + "\x00" + after
	index := c[key]
	c[key] = index + 1
	return index
}

// buildChanges cuts a file's changes into units: by symbol where the language
// can be outlined, by raw hunk where it cannot.
func buildChanges(file *FileChange, hunks []hunk, baseCommit string) []Change {
	beforeSymbols := Outline(file.Path, file.Before)
	afterSymbols := Outline(file.Path, file.After)
	if len(beforeSymbols) == 0 && len(afterSymbols) == 0 {
		return hunkChanges(file, hunks, baseCommit)
	}
	return symbolChanges(file, hunks, beforeSymbols, afterSymbols, baseCommit)
}

// hunkChanges is the fallback for files with no symbol outline.
func hunkChanges(file *FileChange, hunks []hunk, baseCommit string) []Change {
	beforeLines := splitLines(file.Before)
	afterLines := splitLines(file.After)
	occurrences := occurrenceCounter{}
	changes := make([]Change, 0, len(hunks))
	for _, h := range hunks {
		h = trimContext(h, beforeLines, afterLines)
		before := sliceLines(file.Before, h.old)
		after := sliceLines(file.After, h.new)
		changes = append(changes, Change{
			ID:       changeID(baseCommit, file.Path, "", before, after, occurrences.next("", before, after)),
			File:     file.Path,
			Symbol:   nil,
			Kind:     KindBlock,
			Label:    "module level",
			OldRange: h.old,
			NewRange: h.new,
			Before:   before,
			After:    after,
		})
	}
	return changes
}

type pendingUnit struct {
	order  float64
	symbol string
	kind   string
	old    Range
	new    Range
	before string
	after  string
}

func symbolChanges(file *FileChange, hunks []hunk, beforeSymbols, afterSymbols []Symbol, baseCommit string) []Change {
	beforeByName := indexSymbols(beforeSymbols)
	afterByName := indexSymbols(afterSymbols)

	var pending []pendingUnit

	// One unit per symbol whose own lines differ between the two versions.
	names := make([]string, 0, len(afterSymbols)+len(beforeSymbols))
	for _, symbol := range afterSymbols {
		names = append(names, symbol.Name)
	}
	for _, symbol := range beforeSymbols {
		if _, ok := afterByName[symbol.Name]; !ok {
			names = append(names, symbol.Name)
		}
	}
	for _, name := range names {
		beforeSymbol, hadBefore := beforeByName[name]
		afterSymbol, hasAfter := afterByName[name]
		oldRange := symbolRange(beforeSymbol, hadBefore)
		newRange := symbolRange(afterSymbol, hasAfter)
		before := sliceLines(file.Before, oldRange)
		after := sliceLines(file.After, newRange)
		if before == after {
			continue
		}
		kind := afterSymbol.Kind
		if !hasAfter {
			kind = beforeSymbol.Kind
		}
		order := float64(afterSymbol.Start)
		if !hasAfter {
			order = deletedSymbolOrder(name, beforeSymbols, afterByName)
		}
		pending = append(pending, pendingUnit{order, name, kind, oldRange, newRange, before, after})
	}

	// Whatever the hunks touched outside any symbol - imports, constants, top
	// level statements - stays grouped as module level units.
	coveredNew := coveredLines(afterSymbols)
	coveredOld := coveredLines(beforeSymbols)
	beforeLines := splitLines(file.Before)
	afterLines := splitLines(file.After)
	for _, h := range hunks {
		h = trimContext(h, beforeLines, afterLines)
		newRuns := uncoveredRuns(h.new, coveredNew)
		oldRuns := uncoveredRuns(h.old, coveredOld)
		for index := 0; index < len(newRuns) || index < len(oldRuns); index++ {
			newRun, oldRun := Range{Start: 1}, Range{Start: 1}
			if index < len(newRuns) {
				newRun = newRuns[index]
			}
			if index < len(oldRuns) {
				oldRun = oldRuns[index]
			}
			before := sliceLines(file.Before, oldRun)
			after := sliceLines(file.After, newRun)
			if before == after || (isBlank(before) && isBlank(after)) {
				// The blank lines separating two symbols are not a unit.
				continue
			}
			order := float64(newRun.Start)
			if newRun.Empty() {
				order = float64(oldRun.Start)
			}
			pending = append(pending, pendingUnit{order, "", KindBlock, oldRun, newRun, before, after})
		}
	}

	sort.SliceStable(pending, func(i, j int) bool { return pending[i].order < pending[j].order })

	occurrences := occurrenceCounter{}
	changes := make([]Change, 0, len(pending))
	for _, unit := range pending {
		var symbol *string
		label := "module level"
		if unit.symbol != "" {
			name := unit.symbol
			symbol = &name
			label = Symbol{Name: name, Kind: unit.kind}.Label()
		}
		changes = append(changes, Change{
			ID: changeID(baseCommit, file.Path, unit.symbol, unit.before, unit.after,
				occurrences.next(unit.symbol, unit.before, unit.after)),
			File:     file.Path,
			Symbol:   symbol,
			Kind:     unit.kind,
			Label:    label,
			OldRange: unit.old,
			NewRange: unit.new,
			Before:   unit.before,
			After:    unit.after,
		})
	}
	return changes
}

func indexSymbols(symbols []Symbol) map[string]Symbol {
	index := make(map[string]Symbol, len(symbols))
	for _, symbol := range symbols {
		index[symbol.Name] = symbol
	}
	return index
}

func symbolRange(symbol Symbol, present bool) Range {
	if !present {
		return Range{Start: 1, Count: 0}
	}
	return Range{Start: symbol.Start, Count: symbol.End - symbol.Start + 1}
}

// deletedSymbolOrder places a symbol that only exists in the old file just
// after the nearest preceding symbol that survived, so a deleted function is
// read where it used to be instead of being pushed to the end.
func deletedSymbolOrder(name string, beforeSymbols []Symbol, afterByName map[string]Symbol) float64 {
	index := 0
	for i, symbol := range beforeSymbols {
		if symbol.Name == name {
			index = i
			break
		}
	}
	for i := index - 1; i >= 0; i-- {
		if survivor, ok := afterByName[beforeSymbols[i].Name]; ok {
			return float64(survivor.Start) + 0.5
		}
	}
	return 0.5
}

func coveredLines(symbols []Symbol) map[int]bool {
	covered := map[int]bool{}
	for _, symbol := range symbols {
		for line := symbol.Start; line <= symbol.End; line++ {
			covered[line] = true
		}
	}
	return covered
}

// uncoveredRuns collapses the lines of r that no symbol covers into contiguous
// ranges.
func uncoveredRuns(r Range, covered map[int]bool) []Range {
	var runs []Range
	for line := r.Start; line <= r.End(); line++ {
		if covered[line] {
			continue
		}
		if len(runs) > 0 && line == runs[len(runs)-1].End()+1 {
			runs[len(runs)-1].Count++
			continue
		}
		runs = append(runs, Range{Start: line, Count: 1})
	}
	return runs
}

func isBlank(text string) bool {
	for _, char := range text {
		if char != ' ' && char != '\t' && char != '\n' && char != '\r' {
			return false
		}
	}
	return true
}

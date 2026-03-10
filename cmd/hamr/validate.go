package main

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/spf13/cobra"
)

var validateCmd = &cobra.Command{
	Use:   "validate [path]",
	Short: "Check a project for hamr spec compliance",
	Long: `Statically analyse a project directory for hamr compliance.

Checks performed:
  - go.mod exists and declares the hamr dependency
  - At least one call to hamr.New() is present
  - At least one s.Tool() call is present
  - Every Tool() call provides a non-empty name and description literal
  - No obvious misuse patterns detected`,
	Args: cobra.MaximumNArgs(1),
	RunE: runValidate,
}

// ANSI colour helpers — identical palette to dev.go so the CLI is consistent.
const (
	colGreen  = "\033[32m"
	colRed    = "\033[31m"
	colYellow = "\033[33m"
	colBold   = "\033[1m"
	colReset  = "\033[0m"
)

func checkMark() string { return colGreen + "✓" + colReset }
func crossMark() string { return colRed + "✗" + colReset }
func warnMark() string  { return colYellow + "⚠" + colReset }

// finding is a single validation result.
type finding struct {
	ok      bool   // true = pass, false = fail
	warn    bool   // if !ok and warn = true, it is advisory only
	message string
}

func pass(msg string) finding             { return finding{ok: true, message: msg} }
func fail(msg string) finding             { return finding{ok: false, message: msg} }
func warn(msg string) finding             { return finding{ok: false, warn: true, message: msg} }

func runValidate(cmd *cobra.Command, args []string) error {
	root := "."
	if len(args) == 1 {
		root = args[0]
	}

	abs, err := filepath.Abs(root)
	if err != nil {
		return fmt.Errorf("resolve path: %w", err)
	}

	fmt.Printf("%s%sValidating project at %s%s\n\n", colBold, colGreen, abs, colReset)

	findings, fatal := analyseProject(abs)

	passes, fails, warns := 0, 0, 0
	for _, f := range findings {
		switch {
		case f.ok:
			fmt.Printf("  %s  %s\n", checkMark(), f.message)
			passes++
		case f.warn:
			fmt.Printf("  %s  %s\n", warnMark(), f.message)
			warns++
		default:
			fmt.Printf("  %s  %s\n", crossMark(), f.message)
			fails++
		}
	}

	fmt.Printf("\n%s%sSummary:%s %d passed, %d failed, %d warnings\n",
		colBold, colGreen, colReset, passes, fails, warns)

	if fatal || fails > 0 {
		fmt.Printf("\n%s%sProject does not fully comply with the hamr spec.%s\n", colBold, colRed, colReset)
		os.Exit(1)
	}

	if warns > 0 {
		fmt.Printf("\n%s%sProject passes with warnings.%s\n", colBold, colYellow, colReset)
		return nil
	}

	fmt.Printf("\n%s%sProject is compliant.%s\n", colBold, colGreen, colReset)
	return nil
}

// analyseProject runs all checks and returns the collected findings plus a
// fatal flag that is true when a check prevented further analysis.
func analyseProject(root string) ([]finding, bool) {
	var findings []finding

	// --- go.mod checks ---
	goModPath := filepath.Join(root, "go.mod")
	goModContent, err := os.ReadFile(goModPath)
	if err != nil {
		findings = append(findings, fail("go.mod not found — is this a Go module?"))
		return findings, true // can't continue without go.mod
	}
	findings = append(findings, pass("go.mod exists"))

	if strings.Contains(string(goModContent), "github.com/AKhilRaghav0/hamr") {
		findings = append(findings, pass("go.mod imports github.com/AKhilRaghav0/hamr"))
	} else {
		findings = append(findings, fail("go.mod does not import github.com/AKhilRaghav0/hamr"))
	}

	// --- collect all .go source files ---
	goFiles, err := collectGoFiles(root)
	if err != nil {
		findings = append(findings, fail(fmt.Sprintf("failed to walk source tree: %v", err)))
		return findings, true
	}
	if len(goFiles) == 0 {
		findings = append(findings, fail("no .go source files found"))
		return findings, true
	}
	findings = append(findings, pass(fmt.Sprintf("found %d .go source file(s)", len(goFiles))))

	// --- load all source content ---
	sources := loadSources(goFiles)

	// --- hamr.New() call ---
	reNew := regexp.MustCompile(`hamr\.New\s*\(`)
	if anyMatch(reNew, sources) {
		findings = append(findings, pass("hamr.New() call found"))
	} else {
		findings = append(findings, fail("no hamr.New() call found — server is never created"))
	}

	// --- s.Tool() calls ---
	reTool := regexp.MustCompile(`\.Tool\s*\(`)
	toolLines := grepLines(reTool, sources)
	if len(toolLines) == 0 {
		findings = append(findings, warn("no s.Tool() calls found — server has no tools registered"))
	} else {
		findings = append(findings, pass(fmt.Sprintf("%d s.Tool() call(s) found", len(toolLines))))
		findings = append(findings, validateToolCalls(toolLines)...)
	}

	// --- s.Run() or s.RunSSE() ---
	reRun := regexp.MustCompile(`\.Run(SSE)?\s*\(`)
	if anyMatch(reRun, sources) {
		findings = append(findings, pass("s.Run() or s.RunSSE() call found"))
	} else {
		findings = append(findings, fail("no s.Run() or s.RunSSE() call found — server is never started"))
	}

	// --- context.Context import ---
	reCtx := regexp.MustCompile(`"context"`)
	if anyMatch(reCtx, sources) {
		findings = append(findings, pass("\"context\" package imported"))
	} else {
		findings = append(findings, warn("\"context\" package not imported — tool handlers require context.Context as first arg"))
	}

	return findings, false
}

// validateToolCalls inspects each line containing a .Tool( call and checks
// that it provides non-empty string literals for name and description.
func validateToolCalls(lines []string) []finding {
	var findings []finding

	// Match: .Tool("name", "description", ...) — name and description must be
	// non-empty string literals. We accept single or double quotes since some
	// editors may reformat, but Go only uses double quotes.
	reArgs := regexp.MustCompile(`\.Tool\s*\(\s*"([^"]+)"\s*,\s*"([^"]+)"`)

	// Also catch clearly bad patterns: empty first or second string literal.
	reBadName := regexp.MustCompile(`\.Tool\s*\(\s*""\s*,`)
	reBadDesc := regexp.MustCompile(`\.Tool\s*\(\s*"[^"]+"\s*,\s*""`)

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		if reBadName.MatchString(trimmed) {
			findings = append(findings, fail(fmt.Sprintf("Tool() call has empty name: %s", trimmed)))
			continue
		}
		if reBadDesc.MatchString(trimmed) {
			findings = append(findings, fail(fmt.Sprintf("Tool() call has empty description: %s", trimmed)))
			continue
		}

		// If we can parse name + description literals, report them.
		if m := reArgs.FindStringSubmatch(trimmed); m != nil {
			findings = append(findings, pass(fmt.Sprintf("Tool %q has valid name and description", m[1])))
		} else {
			// The call is present but we couldn't parse literals — may span
			// multiple lines or use variables. Warn rather than fail.
			findings = append(findings, warn(fmt.Sprintf("Tool() call could not be statically verified (variables or multiline?): %s", trimmed)))
		}
	}

	return findings
}

// collectGoFiles returns all .go files under root, skipping vendor and testdata.
func collectGoFiles(root string) ([]string, error) {
	var files []string
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		// Skip common non-source directories.
		if info.IsDir() {
			base := info.Name()
			if base == "vendor" || base == "testdata" || base == ".git" {
				return filepath.SkipDir
			}
			return nil
		}
		if filepath.Ext(path) == ".go" {
			files = append(files, path)
		}
		return nil
	})
	return files, err
}

// fileSource pairs a file path with its line-by-line content.
type fileSource struct {
	path  string
	lines []string
}

// loadSources reads all files into memory.
func loadSources(paths []string) []fileSource {
	sources := make([]fileSource, 0, len(paths))
	for _, p := range paths {
		f, err := os.Open(p)
		if err != nil {
			continue
		}
		var lines []string
		sc := bufio.NewScanner(f)
		for sc.Scan() {
			lines = append(lines, sc.Text())
		}
		f.Close()
		sources = append(sources, fileSource{path: p, lines: lines})
	}
	return sources
}

// anyMatch returns true if re matches any line in any source file.
func anyMatch(re *regexp.Regexp, sources []fileSource) bool {
	for _, src := range sources {
		for _, line := range src.lines {
			if re.MatchString(line) {
				return true
			}
		}
	}
	return false
}

// grepLines returns every line (across all files) that matches re.
func grepLines(re *regexp.Regexp, sources []fileSource) []string {
	var out []string
	for _, src := range sources {
		for _, line := range src.lines {
			if re.MatchString(line) {
				out = append(out, line)
			}
		}
	}
	return out
}

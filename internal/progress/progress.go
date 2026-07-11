package progress

import (
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"golang.org/x/term"
)

// ProgressBar wrapper for terminal progress
type ProgressBar struct {
	total            int
	current          int
	description      string
	currentFile      string
	startTime        time.Time
	lastRedraw       time.Time
	throttleDuration time.Duration
	isTerminal       bool
	mu               sync.Mutex
}

// NewProgressBar creates a new progress bar
func NewProgressBar(total int, description string) *ProgressBar {
	isTerm := term.IsTerminal(int(os.Stdout.Fd()))
	pb := &ProgressBar{
		total:            total,
		description:      description,
		startTime:        time.Now(),
		lastRedraw:       time.Unix(0, 0),
		throttleDuration: 65 * time.Millisecond,
		isTerminal:       isTerm,
	}
	pb.drawNoLock()
	return pb
}

func getTerminalWidth() int {
	width, _, err := term.GetSize(int(os.Stdout.Fd()))
	if err != nil {
		return 80 // fallback
	}
	return width
}

func formatTime(d time.Duration) string {
	d = d.Round(time.Second)
	h := d / time.Hour
	d -= h * time.Hour
	m := d / time.Minute
	d -= m * time.Minute
	s := d / time.Second
	if h > 0 {
		return fmt.Sprintf("%02d:%02d:%02d", h, m, s)
	}
	return fmt.Sprintf("%02d:%02d", m, s)
}

func truncateString(s string, maxLen int) string {
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}
	if maxLen <= 3 {
		return string(runes[:maxLen])
	}
	half := (maxLen - 3) / 2
	return string(runes[:half]) + "..." + string(runes[len(runes)-half:])
}

func (pb *ProgressBar) drawNoLock() {
	if !pb.isTerminal {
		// For non-terminal, only log at most once every 5 seconds or on completion
		if pb.current == pb.total || time.Since(pb.lastRedraw) >= 5*time.Second {
			pct := 0
			if pb.total > 0 {
				pct = int(float64(pb.current) * 100 / float64(pb.total))
			}
			fmt.Printf("⚙ %s... %d%% (%d/%d)\n", pb.description, pct, pb.current, pb.total)
			pb.lastRedraw = time.Now()
		}
		return
	}

	termWidth := getTerminalWidth()

	// 1. Construct Line 1 (Progress Bar)
	pct := 0
	if pb.total > 0 {
		pct = int(float64(pb.current) * 100 / float64(pb.total))
	}
	pctStr := fmt.Sprintf("%3d%% ", pct)
	pctLen := len(pctStr)

	elapsed := time.Since(pb.startTime)
	speed := 0
	if elapsed.Seconds() > 0.1 {
		speed = int(float64(pb.current) / elapsed.Seconds())
	}

	totalDigits := len(fmt.Sprintf("%d", pb.total))
	if totalDigits < 1 {
		totalDigits = 1
	}

	countersFormat := fmt.Sprintf(" (%%%dd/%%d, %%d img/s) ", totalDigits)
	countersStr := fmt.Sprintf(countersFormat, pb.current, pb.total, speed)
	countersLen := len(countersStr)

	var remaining time.Duration
	if pb.current > 0 && pb.total > pb.current {
		remaining = time.Duration(float64(elapsed) / float64(pb.current) * float64(pb.total-pb.current))
	}
	timeStr := fmt.Sprintf("[%s<%s]", formatTime(elapsed), formatTime(remaining))
	timeLen := len(timeStr)

	prefixText := "⚙ " + pb.description + "... "
	prefixLen := 2 + len(pb.description) + 4

	barWidth := 30
	showBar := true

	// Step 1: Shrink bar width if needed
	needed := (prefixLen + pctLen + (barWidth + 2) + countersLen + timeLen) - (termWidth - 2)
	if needed > 0 {
		shrinkage := needed
		if barWidth-shrinkage >= 10 {
			barWidth -= shrinkage
			needed = 0
		} else {
			needed -= (barWidth - 10)
			barWidth = 10
		}
	}

	// Step 2: Drop speed from counters if needed
	if needed > 0 {
		countersFormatShort := fmt.Sprintf(" (%%%dd/%%d) ", totalDigits)
		countersStr = fmt.Sprintf(countersFormatShort, pb.current, pb.total)
		oldLen := countersLen
		countersLen = len(countersStr)
		needed -= (oldLen - countersLen)
	}

	// Step 3: Drop time block if needed
	if needed > 0 {
		needed -= timeLen
		timeStr = ""
		timeLen = 0
	}

	// Step 4: Truncate prefix if needed
	if needed > 0 {
		desc := pb.description
		descLimit := len(desc) - needed
		if descLimit < 4 {
			descLimit = 4
		}
		if descLimit < len(desc) {
			desc = desc[:descLimit]
			prefixText = "⚙ " + desc + "... "
			oldLen := prefixLen
			prefixLen = 2 + len(desc) + 4
			needed -= (oldLen - prefixLen)
		}
	}

	// Step 5: Drop bar entirely if needed
	if needed > 0 {
		showBar = false
		needed -= (barWidth + 2)
	}

	// Assemble Line 1
	var line1 string
	line1 += "\033[36m" + prefixText + "\033[0m"
	line1 += pctStr

	if showBar {
		filled := 0
		if pb.total > 0 {
			filled = int(float64(pb.current) / float64(pb.total) * float64(barWidth))
		}
		if filled < 0 {
			filled = 0
		}
		if filled > barWidth {
			filled = barWidth
		}

		line1 += "\033[36m▕\033[0m"
		line1 += "\033[36m" + strings.Repeat("█", filled) + "\033[0m"
		line1 += strings.Repeat("░", barWidth-filled)
		line1 += "\033[36m▏\033[0m"
	}

	line1 += countersStr
	if timeStr != "" {
		line1 += timeStr
	}

	// 2. Construct Line 2 (Status / Current File)
	desc := pb.currentFile
	maxLineLen := termWidth - 5
	if maxLineLen < 20 {
		maxLineLen = 20
	}

	truncated := desc
	if len([]rune(desc)) > maxLineLen {
		prefix := ""
		content := desc
		if strings.HasPrefix(desc, "✓ ") {
			prefix = "✓ "
			content = desc[len("✓ "):]
		} else if strings.HasPrefix(desc, "SKIP ") {
			prefix = "SKIP "
			content = desc[len("SKIP "):]
		} else if strings.HasPrefix(desc, "✗ ") {
			prefix = "✗ "
			content = desc[len("✗ "):]
		} else if strings.HasPrefix(desc, "Processing ") {
			prefix = "Processing "
			content = desc[len("Processing "):]
		}

		adjMaxLen := maxLineLen - len([]rune(prefix))
		if adjMaxLen < 10 {
			adjMaxLen = 10
		}

		truncated = prefix + truncateString(content, adjMaxLen)
	}

	ansiColorized := ""
	if strings.HasPrefix(truncated, "✓ ") {
		ansiColorized = "\033[32m✓\033[0m \033[36m" + truncated[len("✓ "):] + "\033[0m"
	} else if strings.HasPrefix(truncated, "SKIP ") {
		ansiColorized = "\033[33mSKIP\033[0m \033[36m" + truncated[len("SKIP "):] + "\033[0m"
	} else if strings.HasPrefix(truncated, "✗ ") {
		ansiColorized = "\033[31m✗\033[0m \033[36m" + truncated[len("✗ "):] + "\033[0m"
	} else {
		ansiColorized = "\033[36m" + truncated + "\033[0m"
	}

	fmt.Printf("\r%s\n\033[2K%s\033[A\r", line1, ansiColorized)
	pb.lastRedraw = time.Now()
}

// UpdateWithStatus updates the progress bar with a status message and increments
func (pb *ProgressBar) UpdateWithStatus(status string) {
	pb.mu.Lock()
	defer pb.mu.Unlock()

	if pb.isTerminal {
		fmt.Print("\r\033[2K\n\033[2K\033[A\r")
	}
	if status != "" {
		fmt.Println(status)
	}
	pb.current++
	pb.drawNoLock()
}

// Increment just increments the progress bar
func (pb *ProgressBar) Increment() {
	pb.mu.Lock()
	defer pb.mu.Unlock()
	pb.current++
	if time.Since(pb.lastRedraw) >= pb.throttleDuration || pb.current == pb.total {
		pb.drawNoLock()
	}
}

// IncrementWithStatus updates description and increments the progress bar
func (pb *ProgressBar) IncrementWithStatus(status string) {
	pb.mu.Lock()
	defer pb.mu.Unlock()
	pb.currentFile = status
	pb.current++
	if time.Since(pb.lastRedraw) >= pb.throttleDuration || pb.current == pb.total {
		pb.drawNoLock()
	}
}

// Describe sets the description without incrementing
func (pb *ProgressBar) Describe(desc string) {
	pb.mu.Lock()
	defer pb.mu.Unlock()
	pb.currentFile = desc
	if time.Since(pb.lastRedraw) >= pb.throttleDuration {
		pb.drawNoLock()
	}
}

// Finish finishes the progress bar
func (pb *ProgressBar) Finish() {
	pb.mu.Lock()
	defer pb.mu.Unlock()

	if !pb.isTerminal {
		pct := 0
		if pb.total > 0 {
			pct = int(float64(pb.current) * 100 / float64(pb.total))
		}
		fmt.Printf("⚙ %s... %d%% (%d/%d) - Complete\n", pb.description, pct, pb.current, pb.total)
		return
	}

	pb.drawNoLock()
	fmt.Printf("\n\n")
}

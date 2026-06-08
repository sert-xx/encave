package cli

import (
	"fmt"
	"os"

	"golang.org/x/term"
)

// menuAction is the outcome of interpreting a key in the arrow-key menu.
type menuAction int

const (
	menuNone    menuAction = iota // cursor moved or no-op; keep going
	menuConfirm                   // Enter
	menuCancel                    // q / Ctrl-C / Esc / EOF
)

// menuKey interprets a key/escape sequence (the bytes from one Read) against a
// menu of count items at cursor cur, returning the new cursor and the action.
// Supports ↑/↓ (ESC [ A / B), k/j, Enter, and q/Ctrl-C/Esc to cancel.
func menuKey(b []byte, cur, count int) (int, menuAction) {
	if len(b) == 0 {
		return cur, menuCancel
	}
	switch b[0] {
	case '\r', '\n':
		return cur, menuConfirm
	case 3, 'q': // Ctrl-C, q
		return cur, menuCancel
	case 'k': // vim up
		if cur > 0 {
			cur--
		}
		return cur, menuNone
	case 'j': // vim down
		if cur < count-1 {
			cur++
		}
		return cur, menuNone
	case 0x1b: // ESC or an arrow escape sequence
		if len(b) >= 3 && b[1] == '[' {
			switch b[2] {
			case 'A': // up
				if cur > 0 {
					cur--
				}
			case 'B': // down
				if cur < count-1 {
					cur++
				}
			}
			return cur, menuNone
		}
		return cur, menuCancel // bare Esc
	}
	return cur, menuNone
}

// arrowSelect shows an interactive ↑/↓ menu over labels and returns the chosen
// index. A non-nil error means an interactive menu can't be shown (stdin isn't a
// terminal or raw mode failed) — the caller should fall back to a typed prompt.
// ok == false with a nil error means the user cancelled.
func arrowSelect(header string, labels []string) (idx int, ok bool, err error) {
	if len(labels) == 0 {
		return 0, false, nil
	}
	fd := int(os.Stdin.Fd())
	if !term.IsTerminal(fd) {
		return 0, false, fmt.Errorf("stdin is not a terminal")
	}
	oldState, err := term.MakeRaw(fd)
	if err != nil {
		return 0, false, err
	}
	defer term.Restore(fd, oldState)

	if header != "" {
		fmt.Fprintf(os.Stdout, "%s\r\n", header)
	}
	cur := 0
	draw := func() {
		for i, l := range labels {
			pointer := "  "
			text := l
			if i == cur {
				pointer = "> "
				text = "\x1b[7m" + l + "\x1b[0m" // reverse video
			}
			fmt.Fprintf(os.Stdout, "\x1b[2K%s%s\r\n", pointer, text)
		}
	}
	draw()

	buf := make([]byte, 64)
	for {
		n, rerr := os.Stdin.Read(buf)
		if rerr != nil || n == 0 {
			fmt.Fprint(os.Stdout, "\r\n")
			return 0, false, nil
		}
		// A single read may carry several key events; process each in order.
		done := false
		var result int
		var resultOK bool
		for i := 0; i < n && !done; {
			ev, w := nextKeyEvent(buf[i:n])
			i += w
			newCur, action := menuKey(ev, cur, len(labels))
			cur = newCur
			switch action {
			case menuConfirm:
				result, resultOK, done = cur, true, true
			case menuCancel:
				result, resultOK, done = 0, false, true
			}
		}
		if done {
			fmt.Fprint(os.Stdout, "\r\n")
			return result, resultOK, nil
		}
		// Move back up to the first item line and redraw.
		fmt.Fprintf(os.Stdout, "\x1b[%dA", len(labels))
		draw()
	}
}

// nextKeyEvent splits one key event off the front of b and returns it plus the
// number of bytes consumed. An ESC '[' X arrow sequence is 3 bytes; everything
// else (including a lone ESC) is a single byte.
func nextKeyEvent(b []byte) ([]byte, int) {
	if len(b) >= 3 && b[0] == 0x1b && b[1] == '[' {
		return b[:3], 3
	}
	return b[:1], 1
}

// selectFromList lets the user pick an entry from labels, preferring an
// interactive ↑/↓ menu and falling back to a numbered prompt when no menu can be
// shown. Returns the chosen index and ok=false if cancelled.
func selectFromList(header string, labels []string) (int, bool) {
	if idx, ok, err := arrowSelect(header, labels); err == nil {
		return idx, ok
	}
	return numberedSelect(header, labels)
}

package server

import (
	"fmt"
	"strings"
	"unicode"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/uniseg"
)

// ticketFormResult is what the interactive `kanban ticket create` prompt
// collects: a required title and an optional markdown body.
type ticketFormResult struct {
	Title string
	Body  string
}

// promptTicketForm takes over the terminal with a tcell screen, runs the
// ticket form, and returns what the user entered. ok is false when the user
// cancelled (Esc / Ctrl+C). initialBody pre-fills the description (from a
// --body flag given without --title).
func promptTicketForm(boardLabel, initialBody string) (res ticketFormResult, ok bool, err error) {
	screen, err := tcell.NewScreen()
	if err != nil {
		return res, false, fmt.Errorf("open terminal: %w", err)
	}
	if err := screen.Init(); err != nil {
		return res, false, fmt.Errorf("init terminal: %w", err)
	}
	// Fini restores the terminal; make sure it runs even if the loop panics,
	// otherwise the shell is left in raw mode with the alternate screen up.
	defer func() {
		screen.Fini()
		if r := recover(); r != nil {
			panic(r)
		}
	}()
	return runTicketForm(screen, newTicketForm(boardLabel, "", initialBody))
}

// runTicketForm is the event loop, split from promptTicketForm so tests can
// drive it with a tcell.SimulationScreen.
func runTicketForm(screen tcell.Screen, f *ticketForm) (ticketFormResult, bool, error) {
	screen.EnablePaste()
	for {
		f.render(screen)
		screen.Show()
		switch ev := screen.PollEvent().(type) {
		case nil:
			// Screen was finalized underneath us.
			return ticketFormResult{}, false, nil
		case *tcell.EventResize:
			screen.Sync()
		case *tcell.EventPaste:
			f.pasting = ev.Start()
		case *tcell.EventKey:
			f.handleKey(ev)
		}
		if f.submitted {
			return f.result(), true, nil
		}
		if f.cancelled {
			return ticketFormResult{}, false, nil
		}
	}
}

// ---------- model ----------

const (
	focusTitle = iota
	focusBody
)

// ticketForm is the state behind the prompt: two editable fields, which one
// has focus, and the outcome. It is deliberately independent of the screen
// so key handling can be unit-tested without a terminal.
type ticketForm struct {
	boardLabel string
	title      *textBuffer
	body       *textBuffer
	focus      int
	errMsg     string
	pasting    bool
	submitted  bool
	cancelled  bool

	// Scroll offsets are owned by render(): the title scrolls horizontally
	// (in cells), the body vertically (in wrapped visual rows).
	titleScroll int
	bodyScroll  int
}

func newTicketForm(boardLabel, title, body string) *ticketForm {
	f := &ticketForm{
		boardLabel: boardLabel,
		title:      newTextBuffer(title, false),
		body:       newTextBuffer(body, true),
	}
	if strings.TrimSpace(title) != "" && strings.TrimSpace(body) == "" {
		f.focus = focusBody
	}
	return f
}

func (f *ticketForm) focused() *textBuffer {
	if f.focus == focusBody {
		return f.body
	}
	return f.title
}

func (f *ticketForm) result() ticketFormResult {
	return ticketFormResult{
		Title: strings.TrimSpace(f.title.String()),
		Body:  strings.TrimSpace(f.body.String()),
	}
}

func (f *ticketForm) submit() {
	if strings.TrimSpace(f.title.String()) == "" {
		f.errMsg = "a title is required"
		f.focus = focusTitle
		return
	}
	f.submitted = true
}

// handleKey applies one key event. Bindings:
//
//	Tab / Shift+Tab      switch field (Enter or Ctrl+J in the title also moves down)
//	Ctrl+S               submit
//	Esc / Ctrl+C         cancel
//	arrows, Home/End     move; Up from the body's first line returns to the title
//	Ctrl+A / Ctrl+E      line start / end
//	Ctrl+U / Ctrl+K      kill to line start / end
//	Ctrl+W               delete the previous word
//
// While a bracketed paste is in flight, Enter in the title is dropped instead
// of switching fields so a pasted trailing newline doesn't move the cursor.
func (f *ticketForm) handleKey(ev *tcell.EventKey) {
	f.errMsg = ""
	buf := f.focused()
	switch ev.Key() {
	case tcell.KeyEscape, tcell.KeyCtrlC:
		f.cancelled = true
	case tcell.KeyCtrlS:
		f.submit()
	case tcell.KeyTab:
		if f.pasting {
			buf.insert('\t')
			return
		}
		f.focus = (f.focus + 1) % 2
	case tcell.KeyBacktab:
		f.focus = (f.focus + 1) % 2
	case tcell.KeyEnter, tcell.KeyCtrlJ:
		if f.focus == focusTitle {
			if !f.pasting {
				f.focus = focusBody
			}
			return
		}
		buf.newline()
	case tcell.KeyBackspace, tcell.KeyBackspace2:
		buf.backspace()
	case tcell.KeyDelete:
		buf.del()
	case tcell.KeyLeft:
		buf.left()
	case tcell.KeyRight:
		buf.right()
	case tcell.KeyUp:
		if f.focus == focusBody && f.body.row == 0 {
			f.focus = focusTitle
			return
		}
		buf.up()
	case tcell.KeyDown:
		if f.focus == focusTitle {
			f.focus = focusBody
			return
		}
		buf.down()
	case tcell.KeyHome, tcell.KeyCtrlA:
		buf.home()
	case tcell.KeyEnd, tcell.KeyCtrlE:
		buf.end()
	case tcell.KeyCtrlK:
		buf.killToEnd()
	case tcell.KeyCtrlU:
		buf.killToStart()
	case tcell.KeyCtrlW:
		buf.deleteWordBack()
	case tcell.KeyRune:
		if ev.Modifiers()&(tcell.ModCtrl|tcell.ModAlt|tcell.ModMeta) != 0 {
			return
		}
		buf.insert(ev.Rune())
	}
}

// ---------- text buffer ----------

// textBuffer is a small rune-based editor: logical lines plus a cursor.
// Single-line buffers turn newlines into spaces.
type textBuffer struct {
	lines     [][]rune
	row, col  int
	multiline bool
}

func newTextBuffer(initial string, multiline bool) *textBuffer {
	initial = strings.ReplaceAll(initial, "\r\n", "\n")
	if !multiline {
		initial = strings.ReplaceAll(initial, "\n", " ")
	}
	b := &textBuffer{multiline: multiline}
	for _, l := range strings.Split(initial, "\n") {
		b.lines = append(b.lines, []rune(l))
	}
	b.row = len(b.lines) - 1
	b.col = len(b.lines[b.row])
	return b
}

func (b *textBuffer) String() string {
	parts := make([]string, len(b.lines))
	for i, l := range b.lines {
		parts[i] = string(l)
	}
	return strings.Join(parts, "\n")
}

func (b *textBuffer) line() []rune { return b.lines[b.row] }

func (b *textBuffer) clampCol() {
	if n := len(b.line()); b.col > n {
		b.col = n
	}
}

func (b *textBuffer) insert(r rune) {
	if r == '\n' || r == '\r' {
		if b.multiline {
			b.newline()
		} else {
			b.insert(' ')
		}
		return
	}
	line := b.line()
	line = append(line, 0)
	copy(line[b.col+1:], line[b.col:])
	line[b.col] = r
	b.lines[b.row] = line
	b.col++
}

func (b *textBuffer) newline() {
	if !b.multiline {
		return
	}
	line := b.line()
	rest := append([]rune(nil), line[b.col:]...)
	b.lines[b.row] = line[:b.col:b.col]
	b.lines = append(b.lines, nil)
	copy(b.lines[b.row+2:], b.lines[b.row+1:])
	b.lines[b.row+1] = rest
	b.row++
	b.col = 0
}

func (b *textBuffer) backspace() {
	if b.col > 0 {
		line := b.line()
		b.lines[b.row] = append(line[:b.col-1], line[b.col:]...)
		b.col--
		return
	}
	if b.row == 0 {
		return
	}
	prev := b.lines[b.row-1]
	b.col = len(prev)
	b.lines[b.row-1] = append(prev, b.line()...)
	b.lines = append(b.lines[:b.row], b.lines[b.row+1:]...)
	b.row--
}

func (b *textBuffer) del() {
	line := b.line()
	if b.col < len(line) {
		b.lines[b.row] = append(line[:b.col], line[b.col+1:]...)
		return
	}
	if b.row+1 < len(b.lines) {
		b.lines[b.row] = append(line, b.lines[b.row+1]...)
		b.lines = append(b.lines[:b.row+1], b.lines[b.row+2:]...)
	}
}

func (b *textBuffer) left() {
	if b.col > 0 {
		b.col--
	} else if b.row > 0 {
		b.row--
		b.col = len(b.line())
	}
}

func (b *textBuffer) right() {
	if b.col < len(b.line()) {
		b.col++
	} else if b.row+1 < len(b.lines) {
		b.row++
		b.col = 0
	}
}

func (b *textBuffer) up() {
	if b.row > 0 {
		b.row--
		b.clampCol()
	}
}

func (b *textBuffer) down() {
	if b.row+1 < len(b.lines) {
		b.row++
		b.clampCol()
	}
}

func (b *textBuffer) home() { b.col = 0 }
func (b *textBuffer) end()  { b.col = len(b.line()) }

func (b *textBuffer) killToEnd() { b.lines[b.row] = b.line()[:b.col] }

func (b *textBuffer) killToStart() {
	b.lines[b.row] = append([]rune(nil), b.line()[b.col:]...)
	b.col = 0
}

func (b *textBuffer) deleteWordBack() {
	line := b.line()
	i := b.col
	for i > 0 && unicode.IsSpace(line[i-1]) {
		i--
	}
	for i > 0 && !unicode.IsSpace(line[i-1]) {
		i--
	}
	b.lines[b.row] = append(line[:i], line[b.col:]...)
	b.col = i
}

// ---------- rendering ----------

const (
	formPad          = 1
	minFormWidth     = 24
	minFormHeight    = 12
	titleLabelRow    = 2
	titleBoxRow      = 3
	bodyLabelRow     = 7
	bodyBoxRow       = 8
	formFooterRows   = 3 // blank + help + error
	minBodyBoxHeight = 3
)

func (f *ticketForm) render(s tcell.Screen) {
	s.Clear()
	w, h := s.Size()
	base := tcell.StyleDefault
	if w < minFormWidth || h < minFormHeight {
		putText(s, 0, 0, w, base, "terminal too small for the ticket form")
		s.HideCursor()
		return
	}

	putText(s, formPad, 0, w-2*formPad, base.Bold(true), "New ticket · "+f.boardLabel)

	boxX, boxW := formPad, w-2*formPad
	inner := boxW - 2
	label := base.Dim(f.focus != focusTitle)
	putText(s, formPad, titleLabelRow, boxW, label, "Title")
	drawBox(s, boxX, titleBoxRow, boxW, 3, f.focus == focusTitle)
	cursorX, cursorY := f.renderTitle(s, boxX+1, titleBoxRow+1, inner)

	label = base.Dim(f.focus != focusBody)
	putText(s, formPad, bodyLabelRow, boxW, label, "Description (optional, markdown)")
	bodyH := h - formFooterRows - bodyBoxRow
	if bodyH < minBodyBoxHeight {
		bodyH = minBodyBoxHeight
	}
	drawBox(s, boxX, bodyBoxRow, boxW, bodyH, f.focus == focusBody)
	bx, by := f.renderBody(s, boxX+1, bodyBoxRow+1, inner, bodyH-2)
	if f.focus == focusBody {
		cursorX, cursorY = bx, by
	}

	putText(s, formPad, h-2, boxW, base.Dim(true), "Tab switch field · Ctrl+S create ticket · Esc cancel")
	if f.errMsg != "" {
		putText(s, formPad, h-1, boxW, base.Foreground(tcell.ColorRed).Bold(true), f.errMsg)
	}
	s.ShowCursor(cursorX, cursorY)
}

// renderTitle draws the single-line title with horizontal scrolling so the
// cursor stays inside the field. Returns the cursor's screen position.
func (f *ticketForm) renderTitle(s tcell.Screen, x, y, width int) (int, int) {
	line := f.title.lines[0]
	cursor := runesWidth(line[:f.title.col])
	if cursor < f.titleScroll {
		f.titleScroll = cursor
	}
	if cursor >= f.titleScroll+width {
		f.titleScroll = cursor - width + 1
	}
	pos := 0
	for _, r := range line {
		rw := runeWidth(r)
		if pos >= f.titleScroll && pos+rw <= f.titleScroll+width {
			s.SetContent(x+pos-f.titleScroll, y, displayRune(r), nil, tcell.StyleDefault)
		}
		pos += rw
	}
	return x + cursor - f.titleScroll, y
}

// visualRow is one screen row of the soft-wrapped body: a slice
// [start,end) of logical line `line`.
type visualRow struct {
	line       int
	start, end int
}

// wrapLines soft-wraps logical lines at `width` cells. A line whose width
// is an exact multiple of the field width gets a trailing empty row so a
// cursor at its end has somewhere to sit.
func wrapLines(lines [][]rune, width int) []visualRow {
	var rows []visualRow
	for i, line := range lines {
		start, pos := 0, 0
		for j, r := range line {
			rw := runeWidth(r)
			if pos+rw > width && j > start {
				rows = append(rows, visualRow{line: i, start: start, end: j})
				start, pos = j, 0
			}
			pos += rw
		}
		rows = append(rows, visualRow{line: i, start: start, end: len(line)})
		if pos >= width && len(line) > 0 {
			rows = append(rows, visualRow{line: i, start: len(line), end: len(line)})
		}
	}
	return rows
}

// renderBody draws the wrapped body with vertical scrolling. Returns the
// cursor's screen position.
func (f *ticketForm) renderBody(s tcell.Screen, x, y, width, height int) (int, int) {
	if width < 1 || height < 1 {
		return x, y
	}
	rows := wrapLines(f.body.lines, width)
	cursorRow, cursorCol := 0, 0
	for i, vr := range rows {
		if vr.line != f.body.row {
			continue
		}
		if f.body.col >= vr.start && (f.body.col < vr.end || (f.body.col == vr.end && (i+1 == len(rows) || rows[i+1].line != vr.line))) {
			cursorRow = i
			cursorCol = runesWidth(f.body.lines[vr.line][vr.start:f.body.col])
			break
		}
	}
	if cursorRow < f.bodyScroll {
		f.bodyScroll = cursorRow
	}
	if cursorRow >= f.bodyScroll+height {
		f.bodyScroll = cursorRow - height + 1
	}
	if f.bodyScroll > len(rows)-height {
		f.bodyScroll = max(0, len(rows)-height)
	}
	for i := f.bodyScroll; i < len(rows) && i-f.bodyScroll < height; i++ {
		vr := rows[i]
		pos := 0
		for _, r := range f.body.lines[vr.line][vr.start:vr.end] {
			s.SetContent(x+pos, y+i-f.bodyScroll, displayRune(r), nil, tcell.StyleDefault)
			pos += runeWidth(r)
		}
	}
	return x + cursorCol, y + cursorRow - f.bodyScroll
}

// drawBox draws a single-line border. The focused box is bold, others dim,
// so the form reads the same on light and dark terminals without relying
// on a color palette.
func drawBox(s tcell.Screen, x, y, w, h int, focused bool) {
	style := tcell.StyleDefault.Bold(focused).Dim(!focused)
	for i := 1; i < w-1; i++ {
		s.SetContent(x+i, y, '─', nil, style)
		s.SetContent(x+i, y+h-1, '─', nil, style)
	}
	for j := 1; j < h-1; j++ {
		s.SetContent(x, y+j, '│', nil, style)
		s.SetContent(x+w-1, y+j, '│', nil, style)
	}
	s.SetContent(x, y, '┌', nil, style)
	s.SetContent(x+w-1, y, '┐', nil, style)
	s.SetContent(x, y+h-1, '└', nil, style)
	s.SetContent(x+w-1, y+h-1, '┘', nil, style)
}

// putText writes text at (x, y), clipped to maxWidth cells.
func putText(s tcell.Screen, x, y, maxWidth int, style tcell.Style, text string) {
	pos := 0
	for _, r := range text {
		rw := runeWidth(r)
		if pos+rw > maxWidth {
			return
		}
		s.SetContent(x+pos, y, r, nil, style)
		pos += rw
	}
}

// displayRune substitutes a visible stand-in for characters that would
// otherwise render as nothing (tabs pasted into the body).
func displayRune(r rune) rune {
	if r == '\t' {
		return ' '
	}
	return r
}

func runeWidth(r rune) int {
	switch {
	case r == '\t':
		return 1
	case r < 0x20:
		return 0
	case r < 0x300:
		return 1
	}
	return uniseg.StringWidth(string(r))
}

func runesWidth(rs []rune) int {
	n := 0
	for _, r := range rs {
		n += runeWidth(r)
	}
	return n
}

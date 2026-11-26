package tui

// The logos of the agent CLIs show in a terminal that has the kitty
// graphics protocol. rexdiff sends each image once, then draws it with
// Unicode placeholders: plain cells that the renderer moves, clips, and
// redraws as text, and that the terminal paints with the image. The kitty
// renderer covers the cells with its pictures, so it paints the logos in
// its pictures itself.
//
// The logos are masks: black shapes on nothing. They take the colour of
// the text, so they follow the theme of the terminal.

import (
	"bytes"
	"embed"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"os"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/ansi/kitty"
	xdraw "golang.org/x/image/draw"
)

//go:embed logos/*.png
var logoFiles embed.FS

// logoCols is the width of a logo, in cells. Two cells are about square.
const logoCols = 2

// logoImageID returns the kitty image ID of the logo of agent i of guideAgents.
// The ID is the 24-bit colour of the placeholder cells, so it is high
// enough to stay clear of the small IDs that other programs pick.
func logoImageID(i int) int { return 0x524400 + i + 1 }

// kittyImages reports whether the terminal shows images with Unicode
// placeholders. tmux keeps the variables of the outer terminal but drops
// the images, so it has no logos.
//
// ponytail: guesses from the environment; query the terminal (a=q) if a
// terminal that it misses asks for logos.
func kittyImages() bool {
	if os.Getenv("TMUX") != "" {
		return false
	}
	switch os.Getenv("TERM_PROGRAM") {
	case "rex", "ghostty":
		return true
	}
	return os.Getenv("KITTY_WINDOW_ID") != "" || os.Getenv("TERM") == "xterm-kitty"
}

// sendLogos returns the command that sends the logos to the terminal in
// the colour of the text, or nil if the terminal has them, shows no
// images, or the kitty renderer paints them.
func (m *model) sendLogos() tea.Cmd {
	if !m.logos || m.logosSent || m.kitty != nil {
		return nil
	}
	m.logosSent = true
	ink := m.fg
	if ink == nil { // the terminal did not tell yet: a grey that reads on both themes
		ink = color.Gray{0x99}
	}
	var b bytes.Buffer
	for i, a := range guideAgents {
		mask := logoImage(a.name)
		if mask == nil {
			continue
		}
		img := image.NewRGBA(mask.Bounds())
		draw.DrawMask(img, img.Rect, image.NewUniform(ink), image.Point{}, mask, mask.Bounds().Min, draw.Src)
		_ = kitty.EncodeGraphics(&b, img, &kitty.Options{ // a bytes.Buffer does not fail
			Action:           kitty.TransmitAndPut,
			Format:           kitty.PNG,
			ID:               logoImageID(i),
			VirtualPlacement: true,
			Columns:          logoCols,
			Rows:             1,
			Quiet:            2, // no reply on the input
			Chunk:            true,
		})
	}
	return tea.Raw(b.String())
}

// recolorLogos sends the logos again after the colour of the text changed.
// An image with the same ID takes the place of the old one.
func (m *model) recolorLogos() tea.Cmd {
	if !m.logosSent {
		return nil
	}
	m.logosSent = false
	return m.sendLogos()
}

// agentLogo returns the cells of the logo of the agent CLI name, or "" if the
// terminal shows no images or the agent has no logo.
func (m *model) agentLogo(name string) string {
	if id := m.agentLogoID(name); id != 0 {
		return logoCells(id, lipgloss.NewStyle())
	}
	return ""
}

// agentLogoID returns the image ID of the logo of the agent CLI name, or 0 if
// the terminal shows no images or the agent has no logo.
func (m *model) agentLogoID(name string) int {
	if !m.logos {
		return 0
	}
	for i, a := range guideAgents {
		if a.name == name {
			return logoImageID(i)
		}
	}
	return 0
}

// logoCells returns the placeholder cells of the image id in style st:
// one row of logoCols cells, each with the diacritics of its row and
// column. The foreground of the cells is the ID.
func logoCells(id int, st lipgloss.Style) string {
	var b strings.Builder
	for col := range logoCols {
		b.WriteRune(kitty.Placeholder)
		b.WriteRune(kitty.Diacritic(0))
		b.WriteRune(kitty.Diacritic(col))
	}
	return st.Foreground(lipgloss.Color(fmt.Sprintf("#%06x", id))).Render(b.String())
}

// logoImage returns the logo of the agent CLI name, or nil.
func logoImage(name string) image.Image {
	data, err := logoFiles.ReadFile("logos/" + name + ".png")
	if err != nil {
		return nil
	}
	img, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		return nil
	}
	return img
}

// logoCell paints cell c in rect of img, in the colour ink, if it is a
// placeholder of a logo, and reports whether it was one. The colour of the
// cell is the image ID, and its third rune is the diacritic of its column.
func (r *raster) logoCell(img *image.RGBA, rect image.Rectangle, c uv.Cell, ink color.RGBA) bool {
	rs := []rune(c.Content)
	if len(rs) == 0 || rs[0] != kitty.Placeholder {
		return false
	}
	col := 0
	for i := range logoCols {
		if len(rs) >= 3 && kitty.Diacritic(i) == rs[2] {
			col = i
		}
	}
	if c.Style.Fg == nil {
		return true
	}
	cr, cg, cb, _ := c.Style.Fg.RGBA()
	id := int(cr>>8)<<16 | int(cg>>8)<<8 | int(cb>>8)
	if mask := r.logoMask(id); mask != nil {
		draw.DrawMask(img, rect, image.NewUniform(ink), image.Point{}, mask, image.Pt(col*r.cw, 0), draw.Over)
	}
	return true
}

// logoMask returns the mask of the logo of image id, logoCols cells by one, or
// nil. The logo is a square in the middle.
func (r *raster) logoMask(id int) *image.RGBA {
	r.mu.Lock()
	defer r.mu.Unlock()
	if m, ok := r.logos[id]; ok {
		return m
	}
	var m *image.RGBA
	if i := id - logoImageID(0); 0 <= i && i < len(guideAgents) {
		if src := logoImage(guideAgents[i].name); src != nil {
			w, side := logoCols*r.cw, min(logoCols*r.cw, r.ch)
			m = image.NewRGBA(image.Rect(0, 0, w, r.ch))
			at := image.Rect((w-side)/2, (r.ch-side)/2, (w+side)/2, (r.ch+side)/2)
			xdraw.CatmullRom.Scale(m, at, src, src.Bounds(), xdraw.Src, nil)
		}
	}
	if r.logos == nil {
		r.logos = map[int]*image.RGBA{}
	}
	r.logos[id] = m
	return m
}

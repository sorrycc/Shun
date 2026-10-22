package main

import (
	"time"

	"github.com/egoist/mygo/ui"
)

// The colors of icon tiles, as System Settings' own.
var (
	tileGray   = ui.Hex("#8E8E93")
	tileBlue   = ui.Hex("#0A84FF")
	tileGreen  = ui.Hex("#30B850")
	tileOrange = ui.Hex("#FF9500")
	tilePurple = ui.Hex("#AF52DE")
	tileRed    = ui.Hex("#FF3B30")
	tileIndigo = ui.Hex("#5856D6")
)

// Motion, short and soft: Settings is a tool, not a show.
var (
	quick  = ui.Animation{Duration: 140 * time.Millisecond, Ease: ui.EaseOut}
	settle = ui.Animation{Spring: ui.SpringOptions{Duration: 380 * time.Millisecond}}
	pageIn = ui.ElementTransition{Duration: 220 * time.Millisecond, Enter: &ui.Motion{Y: 6}}
)

// look is the palette of Settings, made from the theme: pages a shade off
// the window's color, and cards on them, as System Settings has them.
type look struct {
	canvas, card, cardBorder, hover, header, subtle ui.Color
}

func lookOf(t *ui.Theme) look {
	if t.Dark {
		return look{
			canvas:     t.Background,
			card:       t.Text.Alpha(0.045).Over(t.Background),
			cardBorder: t.Text.Alpha(0.08),
			hover:      t.Text.Alpha(0.06),
			header:     t.Text.Alpha(0.03).Over(t.Background),
			subtle:     t.Text.Alpha(0.08),
		}
	}
	return look{
		canvas:     t.Text.Alpha(0.025).Over(t.Background),
		card:       t.Background,
		cardBorder: t.Text.Alpha(0.08),
		hover:      t.Text.Alpha(0.045),
		header:     t.Background,
		subtle:     t.Text.Alpha(0.06),
	}
}

// settingsTab is a tab of the header: its icon over its name.
type settingsTab struct {
	name string
	icon *ui.SVG
}

var settingsTabs = []settingsTab{
	{"Shortcuts", iconCommand},
	{"General", iconSliders},
	{"Insights", iconActivity},
	{"Privacy", iconShield},
}

// tabPill is the identity of the highlight behind the chosen tab, which
// slides to the tab chosen next.
type tabPill struct{}

// header is the window's title bar: its title, the tab's name, beside the
// window controls, over the tabs; it drags the window.
func header(c *ui.Context, selected *int) {
	t := c.Theme()
	l := lookOf(t)
	bar := c.TitleBar()
	ui.Column(c).Background(l.header).DragWindow().Padding(0, 12, 8, 12).Children(func() {
		if bar.Height > 0 {
			inset := max(bar.Left, bar.Right) // centered in the window
			ui.Row(c).Height(bar.Height).Padding(0, inset).Center().Children(func() {
				ui.Text(c, settingsTabs[*selected].name).FontSize(13).FontWeight(600).SingleLine()
			})
		} else {
			ui.Box(c).Height(8)
		}
		tabs := ui.TabsBase(c, selected, len(settingsTabs))
		tabs.List.Gap(2).AlignSelf(ui.Center).Children(func() {
			for i, tab := range settingsTabs {
				on := i == *selected
				el := tabs.Tab(i).Width(76).Padding(6, 4, 5, 4).Radius(8).Cursor(ui.CursorPointer).Label(tab.name)
				hover := ui.Color{}
				if !on && el.Hovered() {
					hover = l.hover
				}
				el.Background(el.AnimateColor("hover", hover, quick))
				fg := t.TextMuted
				if on {
					fg = t.Accent
				}
				fg = el.AnimateColor("fg", fg, quick)
				el.Children(func() {
					if on {
						ui.Box(c.Key("pill")).Absolute().Left(0).Top(0).Right(0).Bottom(0).Radius(8).
							Background(t.Accent.Alpha(0.12)).
							Transition(ui.ElementTransition{Match: tabPill{}, Animation: &settle})
					}
					ui.Column(c).Grow(1).Gap(3).AlignItems(ui.Center).Children(func() {
						ui.Icon(c, tab.icon).FontSize(18).TextColor(fg)
						label := ui.Text(c, tab.name).FontSize(11).TextColor(fg).SingleLine()
						if on {
							label.FontWeight(600)
						}
					})
				})
			}
		})
	})
	ui.Box(c).Height(1).Background(l.cardBorder)
}

// iconTile is a glyph on a rounded square of color, which leads a setting.
func iconTile(c *ui.Context, icon *ui.SVG, color ui.Color) {
	ui.Box(c).Size(26, 26).Radius(7).Center().Shrink(0).
		LinearGradient(ui.LinearGradient{From: color.Mix(ui.RGB(255, 255, 255), 0.12), To: color, Angle: 180}).
		Children(func() {
			ui.Icon(c, icon).FontSize(15).TextColor(ui.RGB(255, 255, 255))
		})
}

// keycaps shows a combo, or the modifiers held, as the keys pressed: a cap
// of each, as on a keyboard.
func keycaps(c *ui.Context, labels []string, fg ui.Color) {
	t := c.Theme()
	l := lookOf(t)
	ui.Row(c).Gap(3).AlignItems(ui.Center).Children(func() {
		for i, k := range labels {
			ui.Box(c.Key(i)).MinWidth(20).Height(20).Padding(0, 5).Radius(5).Center().
				Background(l.card).Border(1, l.cardBorder).Shadow(0, 1, 0, 0, t.Text.Alpha(0.1)).
				Children(func() {
					ui.Text(c, k).FontSize(12).FontWeight(500).TextColor(fg).SingleLine()
				})
		}
	})
}

// statusPill is a word on a tinted pill, with a dot of its color.
func statusPill(c *ui.Context, text string, color ui.Color) {
	ui.Row(c).Gap(5).AlignItems(ui.Center).Padding(2, 8).Radius(999).Background(color.Alpha(0.12)).Shrink(0).
		Children(func() {
			ui.Box(c).Size(6, 6).Radius(3).Background(color)
			ui.Text(c, text).FontSize(11).FontWeight(600).TextColor(color).SingleLine()
		})
}

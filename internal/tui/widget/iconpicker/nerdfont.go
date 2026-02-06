package iconpicker

import (
	"sort"
	"strings"
	"unicode"

	"github.com/ygelfand/plexctl/pkg/nerdfonts"
)

type NerdFontProvider struct {
	iconList []IconInfo
}

func (p *NerdFontProvider) GetIconList() []IconInfo {
	if len(p.iconList) > 0 {
		return p.iconList
	}

	p.iconList = make([]IconInfo, 0, len(nerdfonts.Icons))
	for k, v := range nerdfonts.Icons {
		p.iconList = append(p.iconList, IconInfo{
			Char: v,
			Name: p.normalizeKey(k),
		})
	}

	sort.Slice(p.iconList, func(i, j int) bool {
		return p.iconList[i].Name < p.iconList[j].Name
	})

	return p.iconList
}

func (p *NerdFontProvider) normalizeKey(k string) string {
	// nf-cod-account -> Cod Account
	parts := strings.Split(k, "-")
	var result []string
	for _, part := range parts {
		if part == "nf" {
			continue
		}
		if len(part) > 0 {
			// Handle underscores if any (e.g. activate_breakpoints)
			subParts := strings.Split(part, "_")
			for _, sp := range subParts {
				if len(sp) > 0 {
					runes := []rune(sp)
					runes[0] = unicode.ToUpper(runes[0])
					result = append(result, string(runes))
				}
			}
		}
	}
	return strings.Join(result, " ")
}

func (p *NerdFontProvider) GetCommonIcons() []IconInfo {
	codes := []string{
		"nf-md-home", "nf-cod-settings_gear", "nf-md-movie", "nf-md-television",
		"nf-md-music", "nf-md-folder", "nf-md-play", "nf-md-library",
		"nf-md-star", "nf-md-heart", "nf-md-account", "nf-md-magnify",
		"nf-md-film", "nf-md-popcorn", "nf-md-ticket", "nf-md-podcast",
		"nf-md-camera", "nf-md-image", "nf-md-video", "nf-md-disc",
		"nf-md-album", "nf-md-playlist_play", "nf-md-ghost", "nf-md-fire",
		"nf-md-rocket", "nf-md-cast", "nf-md-clock", "nf-md-cloud",
		"nf-md-eye", "nf-md-history",
	}

	var common []IconInfo
	for _, code := range codes {
		if char, ok := nerdfonts.Icons[code]; ok {
			common = append(common, IconInfo{
				Char: char,
				Name: p.normalizeKey(code),
			})
		}
	}

	// Fallback if some codes aren't found (NF names can change)
	if len(common) == 0 {
		full := p.GetIconList()
		for i := 0; i < 20 && i < len(full); i++ {
			common = append(common, full[i])
		}
	}

	return common
}

func (p *NerdFontProvider) CanSearch() bool {
	return true
}

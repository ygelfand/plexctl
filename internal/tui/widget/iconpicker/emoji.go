package iconpicker

import (
	"sort"
	"strings"

	"github.com/kyokomi/emoji/v2"
)

type EmojiProvider struct{}

func (p *EmojiProvider) GetIconList() []IconInfo {
	var list []IconInfo
	for name, char := range emoji.CodeMap() {
		list = append(list, IconInfo{Char: strings.TrimSpace(char), Name: name})
	}
	sort.Slice(list, func(i, j int) bool {
		return list[i].Name < list[j].Name
	})
	return list
}

func (p *EmojiProvider) GetCommonIcons() []IconInfo {
	codes := []string{
		":clapper:", ":movie_camera:", ":film_frames:", ":projector:", ":vhs:", ":ticket:", ":popcorn:",
		":tv:",
		":musical_note:", ":headphones:", ":radio:", ":studio_microphone:", ":cd:", ":dvd:",
		":camera:", ":framed_picture:",
		":file_folder:", ":open_file_folder:", ":books:", ":card_index_dividers:",
		":star:", ":heart:",
	}
	var common []IconInfo
	for _, c := range codes {
		char := strings.TrimSpace(emoji.Sprint(c))
		if char != c {
			common = append(common, IconInfo{Char: char, Name: c})
		}
	}
	return common
}

func (p *EmojiProvider) CanSearch() bool {
	return true
}

package iconpicker

type ASCIIProvider struct{}

func (p *ASCIIProvider) GetIconList() []IconInfo {
	return []IconInfo{
		{">", "Greater"}, {"<", "Less"}, {"=", "Equal"}, {"-", "Minus"},
		{"+", "Plus"}, {"*", "Star"}, {"~", "Tilde"}, {"^", "Caret"}, {"#", "Hash"},
		{"[", "Left Bracket"}, {"]", "Right Bracket"}, {"(", "Left Paren"}, {")", "Right Paren"},
		{"{", "Left Brace"}, {"}", "Right Brace"}, {"|", "Pipe"}, {"/", "Slash"},
		{"▷", "Play"}, {"▹", "Small Play"}, {"□", "Square"}, {"■", "Full Square"},
		{"▪", "Small Full Square"}, {"▫", "Small Square"}, {"•", "Bullet"}, {"‣", "Bullet 2"},
		{"♪", "Music"}, {"H", "Home"}, {"S", "Settings"}, {"M", "Movie"}, {"T", "TV Show"},
		{"A", "Artist"}, {"P", "Photo"}, {"D", "Directory"}, {"V", "Video"},
	}
}

func (p *ASCIIProvider) GetCommonIcons() []IconInfo {
	return p.GetIconList()
}

func (p *ASCIIProvider) CanSearch() bool {
	return false
}

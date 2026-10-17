package config

func (c *Config) MusicOutput() VideoOutput {
	if c.MusicPlayer == "window" {
		return VideoWindow
	}
	return AudioTerminal
}

func IsMusic(mediaType string) bool {
	return mediaType == "artist" || mediaType == "album" || mediaType == "track"
}

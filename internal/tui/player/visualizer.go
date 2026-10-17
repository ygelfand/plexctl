package player

// musicVideoGraph keeps the audible stream intact while creating a video feed
// for windowed music and explicit terminal video. AudioTerminal keeps its own
// waveform sampler and never loads this graph.
func musicVideoGraph(style string) string {
	const size = "1024x576"
	filter := ""
	switch style {
	case "cqt":
		filter = "showcqt=s=" + size + ":fps=30"
	case "spectrum":
		filter = "showspectrum=s=" + size + ":slide=scroll:color=intensity:fps=30"
	case "waves":
		filter = "showwaves=s=" + size + ":mode=line:rate=30"
	case "scope":
		filter = "avectorscope=s=" + size + ":rate=30:draw=line"
	default:
		return ""
	}
	return "[aid1]asplit[ao][vis];[vis]" + filter + "[vo]"
}

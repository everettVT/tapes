package merkle

type input struct {
	Parent  string `json:"parent"`
	Content any    `json:"content"`
}

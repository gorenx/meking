package agent

type EmbeddingRequest struct {
	Texts []string
}

type EmbeddingResponse struct {
	Vectors [][]float64
}

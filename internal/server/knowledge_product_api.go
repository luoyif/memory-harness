package server

import (
	"net/http"
	"strings"
)

func (s *Server) knowledgeProductBlocks(w http.ResponseWriter, r *http.Request) {
	objectID := strings.TrimSpace(r.PathValue("id"))
	blocks, err := s.app.Growth.ProductBlocks(r.Context(), objectID)
	if err != nil {
		writePortfolioError(w, "knowledge_product_blocks_failed", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"blocks": blocks, "total": len(blocks)})
}

func (s *Server) setKnowledgeProductBlockLocks(w http.ResponseWriter, r *http.Request) {
	var request struct {
		ExpectedRevision int      `json:"expected_revision"`
		BlockIDs         []string `json:"block_ids"`
	}
	if err := decodeBody(r, &request); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_product_block_locks", err.Error())
		return
	}
	blocks, err := s.app.Growth.SetProductBlockLocks(r.Context(), strings.TrimSpace(r.PathValue("id")), request.ExpectedRevision, request.BlockIDs)
	if err != nil {
		writePortfolioError(w, "knowledge_product_block_locks_failed", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"blocks": blocks, "total": len(blocks)})
}

func (s *Server) knowledgeProductMergePreview(w http.ResponseWriter, r *http.Request) {
	preview, err := s.app.Growth.ProductMergePreview(r.Context(), strings.TrimSpace(r.PathValue("id")))
	if err != nil {
		writePortfolioError(w, "knowledge_product_merge_preview_failed", err)
		return
	}
	writeJSON(w, http.StatusOK, preview)
}

package cmd

import (
	"context"
	"fmt"

	"github.com/piranha/qqmd/llm"
	"github.com/piranha/qqmd/store"
	"github.com/spf13/cobra"
)

var embedCmd = &cobra.Command{
	Use:   "embed",
	Short: "Generate vector embeddings for indexed documents",
	Run: func(cmd *cobra.Command, args []string) {
		s, err := openStore()
		if err != nil {
			fatal("opening store: %v", err)
		}
		defer s.Close()

		force, _ := cmd.Flags().GetBool("force")
		provider := llm.NewOllamaProvider()
		ctx := context.Background()

		hashes, err := s.GetAllActiveHashes()
		if err != nil {
			fatal("getting hashes: %v", err)
		}

		var embedded, skipped, failed int

		for _, hash := range hashes {
			if !force && s.HasEmbeddings(hash) {
				skipped++
				continue
			}

			body, err := s.GetContentBody(hash)
			if err != nil {
				failed++
				continue
			}

			// Chunk the document
			chunks := store.ChunkDocument(body, store.DefaultChunkSize, store.DefaultChunkOverlap)

			// Delete old embeddings if force
			if force {
				s.DeleteEmbeddings(hash)
			}

			// Generate embeddings for each chunk
			texts := make([]string, len(chunks))
			for i, chunk := range chunks {
				texts[i] = llm.FormatDocForEmbedding("", chunk.Text)
			}

			embeddings, err := provider.EmbedBatch(ctx, texts)
			if err != nil {
				fmt.Printf("  Failed to embed %s: %v\n", hash[:6], err)
				failed++
				continue
			}

			for i, emb := range embeddings {
				if err := s.StoreEmbedding(hash, i, chunks[i].Start, chunks[i].End, emb, provider.Name()); err != nil {
					fmt.Printf("  Failed to store embedding for %s chunk %d: %v\n", hash[:6], i, err)
				}
			}
			embedded++
			fmt.Printf("  Embedded #%s (%d chunks)\n", hash[:6], len(chunks))
		}

		fmt.Printf("\nDone: %d embedded, %d skipped, %d failed (total: %d)\n",
			embedded, skipped, failed, len(hashes))
	},
}

func init() {
	embedCmd.Flags().BoolP("force", "f", false, "Force re-embedding of all documents")
	rootCmd.AddCommand(embedCmd)
}

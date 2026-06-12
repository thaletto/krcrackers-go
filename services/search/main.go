package search

import (
	"context"
	"fmt"
	"strconv"

	"github.com/meilisearch/meilisearch-go"
)

const indexName = "products"

type ProductDocument struct {
	ID           int64   `json:"id"`
	Name         string  `json:"name"`
	Description  string  `json:"description"`
	Price        float64 `json:"price"`
	Brand        string  `json:"brand"`
	Category     string  `json:"category"`
	Image        string  `json:"image"`
	ComparePrice float64 `json:"compare_price"`
}

type SearchFilters struct {
	Query    string
	Category string
	Brand    string
	MinPrice float64
	MaxPrice float64
	Sort     string
	Limit    int
	Offset   int
}

type SearchResult struct {
	ProductIDs []int
	Total      int
}

type Service interface {
	IndexProduct(ctx context.Context, doc ProductDocument) error
	DeleteProduct(ctx context.Context, id int) error
	Search(ctx context.Context, filters SearchFilters) (SearchResult, error)
	ReindexAll(ctx context.Context, docs []ProductDocument) error
}

type service struct {
	client meilisearch.ServiceManager
}

func NewService(url, apiKey string) (Service, error) {
	client := meilisearch.New(url, meilisearch.WithAPIKey(apiKey))
	idx := client.Index(indexName)
	_, err := idx.UpdateFilterableAttributes(&[]interface{}{"category", "brand", "price"})
	if err != nil {
		return nil, fmt.Errorf("meilisearch filterable attrs: %w", err)
	}
	_, err = idx.UpdateSortableAttributes(&[]string{"price", "id"})
	if err != nil {
		return nil, fmt.Errorf("meilisearch sortable attrs: %w", err)
	}
	return &service{client: client}, nil
}

func (s *service) IndexProduct(ctx context.Context, doc ProductDocument) error {
	_, err := s.client.Index(indexName).AddDocumentsWithContext(ctx, []ProductDocument{doc}, nil)
	return err
}

func (s *service) DeleteProduct(ctx context.Context, id int) error {
	_, err := s.client.Index(indexName).DeleteDocumentWithContext(ctx, strconv.Itoa(id), nil)
	return err
}

func (s *service) Search(ctx context.Context, filters SearchFilters) (SearchResult, error) {
	req := &meilisearch.SearchRequest{}

	var filterParts []string
	if filters.Category != "" {
		filterParts = append(filterParts, fmt.Sprintf("category = %q", filters.Category))
	}
	if filters.Brand != "" {
		filterParts = append(filterParts, fmt.Sprintf("brand = %q", filters.Brand))
	}
	if filters.MinPrice > 0 {
		filterParts = append(filterParts, fmt.Sprintf("price >= %v", filters.MinPrice))
	}
	if filters.MaxPrice > 0 {
		filterParts = append(filterParts, fmt.Sprintf("price <= %v", filters.MaxPrice))
	}
	if len(filterParts) > 0 {
		req.Filter = filterParts
	}

	switch filters.Sort {
	case "price_asc":
		req.Sort = []string{"price:asc"}
	case "price_desc":
		req.Sort = []string{"price:desc"}
	case "newest":
		req.Sort = []string{"id:desc"}
	}

	if filters.Limit > 0 {
		req.Limit = int64(filters.Limit)
	}
	if filters.Offset > 0 {
		req.Offset = int64(filters.Offset)
	}

	resp, err := s.client.Index(indexName).SearchWithContext(ctx, filters.Query, req)
	if err != nil {
		return SearchResult{}, err
	}

	ids := make([]int, 0, len(resp.Hits))
	for _, hit := range resp.Hits {
		var doc ProductDocument
		if err := hit.Decode(&doc); err == nil {
			ids = append(ids, int(doc.ID))
		}
	}

	return SearchResult{
		ProductIDs: ids,
		Total:      int(resp.EstimatedTotalHits),
	}, nil
}

func (s *service) ReindexAll(ctx context.Context, docs []ProductDocument) error {
	if len(docs) == 0 {
		return nil
	}
	_, err := s.client.Index(indexName).AddDocumentsWithContext(ctx, docs, nil)
	return err
}

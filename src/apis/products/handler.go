package products

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	authapi "github.com/thaletto/krcrackers-go/src/apis/auth"
	apperrors "github.com/thaletto/krcrackers-go/src/errors"
	"github.com/thaletto/krcrackers-go/src/server"
	"github.com/thaletto/krcrackers-go/src/services/products"
)

type Handler struct {
	svc *products.Service
}

func NewHandler(svc *products.Service) *Handler {
	return &Handler{svc: svc}
}

func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /products", h.list)
	mux.HandleFunc("GET /products/{id}", h.get)

	adminAuth := func(fn http.HandlerFunc) http.HandlerFunc {
		return authapi.WithAuth(authapi.WithAdmin(fn)).ServeHTTP
	}
	mux.HandleFunc("POST /admin/products", adminAuth(h.create))
	mux.HandleFunc("PUT /admin/products/{id}", adminAuth(h.update))
	mux.HandleFunc("DELETE /admin/products/{id}", adminAuth(h.delete))
}

// create godoc
//
// @Summary      Create a product
// @Description  Create a product as an administrator
// @Tags         products
// @Accept       json
// @Produce      json
// @Param        input  body      products.ProductInput  true  "Product details"
// @Success      201    {object}  products.Product
// @Failure      400    {object}  server.ErrorResponse
// @Failure      401    {object}  server.ErrorResponse
// @Failure      403    {object}  server.ErrorResponse
// @Failure      422    {object}  server.ErrorResponse
// @Router       /admin/products [post]
func (h *Handler) create(w http.ResponseWriter, r *http.Request) {
	var input products.ProductInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		server.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	product, err := h.svc.Create(r.Context(), input)
	if err != nil {
		server.WriteError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}

	server.WriteJSON(w, http.StatusCreated, product)
}

// list godoc
//
// @Summary      List products
// @Description  List products with optional search, filter, sort, and pagination
// @Tags         products
// @Produce      json
// @Param        q          query     string  false  "Search query"
// @Param        category   query     string  false  "Category"
// @Param        brand      query     string  false  "Brand"
// @Param        min_price  query     number  false  "Minimum price"
// @Param        max_price  query     number  false  "Maximum price"
// @Param        sort       query     string  false  "Sort order"
// @Param        limit      query     int     false  "Page limit"
// @Param        offset     query     int     false  "Page offset"
// @Success      200        {object}  products.ListProductsResponse
// @Failure      500        {object}  server.ErrorResponse
// @Router       /products [get]
func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	query := q.Get("q")
	category := q.Get("category")
	brand := q.Get("brand")
	minPrice, _ := strconv.ParseFloat(q.Get("min_price"), 64)
	maxPrice, _ := strconv.ParseFloat(q.Get("max_price"), 64)
	sort := q.Get("sort")
	limit, _ := strconv.Atoi(q.Get("limit"))
	offset, _ := strconv.Atoi(q.Get("offset"))

	hasFilters := query != "" || category != "" || brand != "" || minPrice > 0 || maxPrice > 0 || sort != ""

	var resp products.ListProductsResponse
	var err error

	if hasFilters {
		resp, err = h.svc.Search(r.Context(), products.Filter{
			Query: query, Category: category, Brand: brand,
			MinPrice: minPrice, MaxPrice: maxPrice,
			Sort: sort, Limit: limit, Offset: offset,
		})
	} else {
		resp, err = h.svc.List(r.Context(), limit, offset)
	}
	if err != nil {
		server.WriteError(w, http.StatusInternalServerError, "failed to list products")
		return
	}

	server.WriteJSON(w, http.StatusOK, resp)
}

// get godoc
//
// @Summary      Get a product
// @Description  Get a product by ID
// @Tags         products
// @Produce      json
// @Param        id   path      int  true  "Product ID"
// @Success      200  {object}  products.Product
// @Failure      400  {object}  server.ErrorResponse
// @Failure      404  {object}  server.ErrorResponse
// @Failure      500  {object}  server.ErrorResponse
// @Router       /products/{id} [get]
func (h *Handler) get(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		server.WriteError(w, http.StatusBadRequest, "invalid product id")
		return
	}

	product, err := h.svc.Get(r.Context(), id)
	if err != nil {
		if errors.Is(err, apperrors.ErrNotFound) {
			server.WriteError(w, http.StatusNotFound, err.Error())
			return
		}
		server.WriteError(w, http.StatusInternalServerError, "failed to get product")
		return
	}

	server.WriteJSON(w, http.StatusOK, product)
}

// update godoc
//
// @Summary      Update a product
// @Description  Replace a product as an administrator
// @Tags         products
// @Accept       json
// @Produce      json
// @Param        id     path      int                    true  "Product ID"
// @Param        input  body      products.ProductInput  true  "Product details"
// @Success      200    {object}  products.Product
// @Failure      400    {object}  server.ErrorResponse
// @Failure      401    {object}  server.ErrorResponse
// @Failure      403    {object}  server.ErrorResponse
// @Failure      404    {object}  server.ErrorResponse
// @Failure      422    {object}  server.ErrorResponse
// @Router       /admin/products/{id} [put]
func (h *Handler) update(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		server.WriteError(w, http.StatusBadRequest, "invalid product id")
		return
	}

	var input products.ProductInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		server.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	product, err := h.svc.Update(r.Context(), id, input)
	if err != nil {
		if errors.Is(err, apperrors.ErrNotFound) {
			server.WriteError(w, http.StatusNotFound, err.Error())
			return
		}
		server.WriteError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}

	server.WriteJSON(w, http.StatusOK, product)
}

// delete godoc
//
// @Summary      Delete a product
// @Description  Delete a product as an administrator
// @Tags         products
// @Param        id   path  int  true  "Product ID"
// @Success      204
// @Failure      400  {object}  server.ErrorResponse
// @Failure      401  {object}  server.ErrorResponse
// @Failure      403  {object}  server.ErrorResponse
// @Failure      404  {object}  server.ErrorResponse
// @Failure      500  {object}  server.ErrorResponse
// @Router       /admin/products/{id} [delete]
func (h *Handler) delete(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		server.WriteError(w, http.StatusBadRequest, "invalid product id")
		return
	}

	if err := h.svc.Delete(r.Context(), id); err != nil {
		if errors.Is(err, apperrors.ErrNotFound) {
			server.WriteError(w, http.StatusNotFound, err.Error())
			return
		}
		server.WriteError(w, http.StatusInternalServerError, "failed to delete product")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

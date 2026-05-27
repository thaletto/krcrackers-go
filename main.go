package main
import (
	"encoding/json"
	"fmt"
	"net/http"
)

type User struct {
	ID string `json:"id"`
	Name string `json:"name"`
}

func main() {
	router := http.NewServeMux()

	router.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, "Healthy and ready")
	})

	router.HandleFunc("GET /api/users/{id}", func(w http.ResponseWriter, r *http.Request) {
		userID := r.PathValue("id")
		user := User{
			ID: userID,
			Name: "Laxman",
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(user)
	})

	fmt.Println("Server is running on http://localhost:8000")
	if err := http.ListenAndServe(":8080", router); err != nil {
		fmt.Printf("Server failed to start: %v\n", err)
	}
}
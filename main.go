package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"log"
	"net/http"
	"time"

	"github.com/gorilla/mux"
	_ "github.com/go-sql-driver/mysql"
	"github.com/go-redis/redis/v8"
)

// Article represents a news article.
type Article struct {
	ID          int    `json:"id"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Author      string `json:"author"`
	ImageURL    string `json:"image_url"`
	CreatedAt   string `json:"created_at"`
}

var (
	db          *sql.DB
	redisClient *redis.Client
)

func init() {
	initDB()
	initRedis()
}

func initDB() {
	var err error
	db, err = sql.Open("mysql", "root:pAra123gon@tcp(128.199.124.75:3306)/news_db")
	if err != nil {
		log.Fatal(err)
	}

	if err := db.Ping(); err != nil {
		log.Fatal(err)
	}
}
// replace the ip address to 152.42.164.25 to connect to the redis caching server i just configured. i did not set any password.
func initRedis() {
	redisClient = redis.NewClient(&redis.Options{
		Addr:     "139.59.253.157:6379",
		Password: "",
		DB:       0,
	})

	if _, err := redisClient.Ping(context.Background()).Result(); err != nil {
		log.Fatal(err)
	}
}

func GetAllArticles(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Check if data exists in cache
	cacheData, err := redisClient.Get(ctx, "articles").Bytes()
	if err == nil {
		// Data found in cache
		var articles []Article
		if err := json.Unmarshal(cacheData, &articles); err != nil {
			log.Println(err)
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}
		json.NewEncoder(w).Encode(articles)
		return
	}

	// Data not found in cache, fetch from database
	rows, err := db.QueryContext(ctx, "SELECT * FROM articles")
	if err != nil {
		log.Println(err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var articles []Article
	for rows.Next() {
		var article Article
		if err := rows.Scan(&article.ID, &article.Title, &article.Description, &article.Author, &article.ImageURL, &article.CreatedAt); err != nil {
			log.Println(err)
			continue
		}
		articles = append(articles, article)
	}

	if err := rows.Err(); err != nil {
		log.Println(err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	// Set data to cache with a TTL of 10 minutes
	articlesJSON, err := json.Marshal(articles)
	if err != nil {
		log.Println(err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	if err := redisClient.Set(ctx, "articles", articlesJSON, 15*time.Minute).Err(); err != nil {
		log.Println(err)
	}

	json.NewEncoder(w).Encode(articles)
}

func CreateArticle(w http.ResponseWriter, r *http.Request) {
	var article Article
	if err := json.NewDecoder(r.Body).Decode(&article); err != nil {
		log.Println(err)
		http.Error(w, "Bad Request", http.StatusBadRequest)
		return
	}

	_, err := db.Exec("INSERT INTO articles (title, description, author, image_url, created_at) VALUES (?, ?, ?, ?, ?)", article.Title, article.Description, article.Author, article.ImageURL, time.Now())
	if err != nil {
		log.Println(err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	// Invalidate cache after inserting new article
	if err := redisClient.Del(r.Context(), "articles").Err(); err != nil {
		log.Println(err)
	}

	w.WriteHeader(http.StatusCreated)
}

func DeleteArticle(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	vars := mux.Vars(r)
	articleID := vars["id"]

	// Delete the article from the database
	_, err := db.ExecContext(ctx, "DELETE FROM articles WHERE id = ?", articleID)
	if err != nil {
		log.Println(err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	// Invalidate cache after deleting the article
	if err := redisClient.Del(ctx, "articles").Err(); err != nil {
		log.Println(err)
	}

	w.WriteHeader(http.StatusOK)
}

func main() {
	router := mux.NewRouter()
	router.HandleFunc("/articles", GetAllArticles).Methods("GET")
	router.HandleFunc("/articles", CreateArticle).Methods("POST")
	router.HandleFunc("/articles/{id}", DeleteArticle).Methods("DELETE")

	log.Fatal(http.ListenAndServe(":8080", router))
}

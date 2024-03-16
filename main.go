package main

import (
	"database/sql"
	"encoding/json"
	"log"
	"math"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/gorilla/mux"
	_ "github.com/lib/pq"
)

// Post
type Post struct {
	ID           string    `json:"id"`
	Title        string    `json:"title"`
	Content      string    `json:"content"`
	Published    bool      `json:"published"`
	CreatedAt    time.Time `json:"created_at"`
	ViewCount    int       `json:"-"`
	Updated_at   time.Time `json:"-"`
	StartDateStr time.Time `json:"-"`
	EndDateStr   time.Time `json:"-"`
}

// Response
type Response struct {
	Posts     []Post `json:"posts"`
	Count     int    `json:"count"`
	Limit     int    `json:"limit"`
	Page      int    `json:"page"`
	TotalPage int    `json:"total_page"`
}

// HandleFunc
func main() {
	handleFunc()
}

func handleFunc() {
	router := mux.NewRouter()
	router.HandleFunc("/posts", getPosts).Methods("GET")
	router.HandleFunc("/posts/date", getPostsDate).Methods("GET")
	router.HandleFunc("/posts/{id}", getPostsId).Methods("GET")
	router.HandleFunc("/posts", createPostNew).Methods("POST")
	router.HandleFunc("/posts/dateRange", getPostsDateRange).Methods("POST")
	router.HandleFunc("/posts/{id}", updatePost).Methods("PUT")
	router.HandleFunc("/posts/{id}", deletePost).Methods("DELETE")
	port := os.Getenv("PORT")
	if port == "" {
		port = "3000"
	}
	log.Fatal(http.ListenAndServe(":"+port, router))
}

// connectToDatabase เชื่อมต่อฐานข้อมูล PostgreSQL
func connectToDatabase() (*sql.DB, error) {
	dbURL := "postgres://posts:38S2GPNZut4Tmvan@dev.opensource-technology.com:5523/posts?sslmode=disable"
	db, err := sql.Open("postgres", dbURL)
	if err != nil {
		return nil, err
	}
	return db, nil
}

// respondWithJSON ตอบกลับด้วยข้อมูล JSON
func respondWithJSON(w http.ResponseWriter, data interface{}, statusCode int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	json.NewEncoder(w).Encode(data)
}

// respondWithError ตอบกลับด้วยข้อผิดพลาดและรหัสสถานะ
func respondWithError(w http.ResponseWriter, statusCode int, errorMessage string) {
	w.WriteHeader(statusCode)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"error": errorMessage})
}

func getDataCount() (int, error) {
	db, err := connectToDatabase()
	if err != nil {
		return 0, err
	}
	defer db.Close()

	var count int
	err = db.QueryRow("SELECT COUNT(*) FROM posts WHERE published = true").Scan(&count)
	if err != nil {
		return 0, err
	}

	return count, nil
}

// getData ดึงข้อมูลโพสต์ทั้งหมดที่เผยแพร่จากฐานข้อมูล
func getPosts(w http.ResponseWriter, r *http.Request) {
	// รับพารามิเตอร์จาก query string
	queryParams := r.URL.Query()
	pageStr := queryParams.Get("page")
	limitStr := queryParams.Get("limit")
	title := queryParams.Get("title")

	// แปลงค่า page และ limit เป็น integer โดยใช้ค่าเริ่มต้นถ้าไม่ได้ระบุ
	page, _ := strconv.Atoi(pageStr)
	if page == 0 {
		page = 1
	}
	limit, _ := strconv.Atoi(limitStr)
	if limit == 0 {
		count, err := getDataCount()
		if err != nil {

		} else {
			limit = count
		}
	}
	offset := (page - 1) * limit

	db, err := connectToDatabase()
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer db.Close()

	// ปรับ query SQL เพื่อค้นหาโพสต์
	query := "SELECT id, title, content, published, created_at FROM posts WHERE published = true AND (title LIKE '%' || $1 || '%') LIMIT $2 OFFSET $3"

	rows, err := db.Query(query, title, limit, offset)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()

	var posts []Post

	for rows.Next() {
		var post Post
		err := rows.Scan(&post.ID, &post.Title, &post.Content, &post.Published, &post.CreatedAt)
		if err != nil {
			respondWithError(w, http.StatusInternalServerError, err.Error())
			return
		}
		posts = append(posts, post)
	}

	// คำนวณ Total Page
	var totalPosts int
	err = db.QueryRow("SELECT COUNT(*) FROM posts WHERE published = true AND (title LIKE '%' || $1 || '%')", title).Scan(&totalPosts)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, err.Error())
		return
	}

	var totalPage int

	if totalPosts == 0 {
		totalPage = 1
	} else {
		totalPage = int(math.Ceil(float64(totalPosts) / float64(limit)))
	}

	response := Response{
		Posts:     postsOrDefault(posts),
		Count:     len(posts),
		Limit:     limit,
		Page:      page,
		TotalPage: totalPage,
	}
	respondWithJSON(w, response, http.StatusOK)
}

func postsOrDefault(posts []Post) []Post {
	if posts == nil {
		return []Post{} // ส่งกลับ slice ว่างเมื่อ posts เป็น nil
	}
	return posts
}

// getPostsId ดึง post ด้วย ID จากฐานข้อมูล
func getPostsId(w http.ResponseWriter, r *http.Request) {
	params := mux.Vars(r)
	postID := params["id"]

	db, err := connectToDatabase()
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer db.Close()

	// เพิ่มค่า view_count โดยไม่ต้องรับค่าที่ Return กลับ โดยใช้เงื่อนไข Published = true
	_, err = db.Exec("UPDATE posts SET view_count = view_count + 1 WHERE id = $1 AND published = true", postID)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// ดึงข้อมูล posts ทั้งหมดโดยไม่สนใจ published
	var post Post
	err = db.QueryRow("SELECT id, title, content, published, created_at, view_count FROM posts WHERE id = $1", postID).
		Scan(&post.ID, &post.Title, &post.Content, &post.Published, &post.CreatedAt, &post.ViewCount)
	if err != nil {
		if err == sql.ErrNoRows {
			respondWithError(w, http.StatusNotFound, "this id not found.")
			return
		}
		respondWithError(w, http.StatusInternalServerError, "error message")
		return
	}

	respondWithJSON(w, post, http.StatusOK)
}

func getPostsDate(w http.ResponseWriter, r *http.Request) {
	// รับพารามิเตอร์จาก query string
	queryParams := r.URL.Query()
	pageStr := queryParams.Get("page")
	limitStr := queryParams.Get("limit")
	title := queryParams.Get("title")
	createdAt := queryParams.Get("createdAt")

	// แปลงค่า page และ limit เป็น integer โดยใช้ค่าเริ่มต้นถ้าไม่ได้ระบุ
	page, _ := strconv.Atoi(pageStr)
	if page == 0 {
		page = 1
	}
	limit, _ := strconv.Atoi(limitStr)
	if limit == 0 {
		count, err := getDataCount()
		if err != nil {
		} else {
			limit = count
		}
	}

	// คำนวณ Offset
	offset := (page - 1) * limit

	db, err := connectToDatabase()
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer db.Close()

	// ปรับ query SQL เพื่อค้นหา post ที่ตรงกับ title และ createdAt ที่ระบุ (หรือเป็นส่วนหนึ่งของ title และ createdAt)
	query := "SELECT id, title, content, published, created_at FROM posts WHERE (title LIKE '%' || $1 || '%') AND (created_at::date = $2) LIMIT $3 OFFSET $4"

	rows, err := db.Query(query, title, createdAt, limit, offset)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()

	var posts []Post

	for rows.Next() {
		var post Post
		err := rows.Scan(&post.ID, &post.Title, &post.Content, &post.Published, &post.CreatedAt)
		if err != nil {
			respondWithError(w, http.StatusInternalServerError, err.Error())
			return
		}
		posts = append(posts, post)
	}

	if len(posts) == 0 {
		respondWithError(w, http.StatusNotFound, "The searched post was not found.")
		return
	}

	// คำนวณ Total Page
	var totalPosts int
	err = db.QueryRow("SELECT COUNT(*) FROM posts WHERE published = true AND (title LIKE '%' || $1 || '%') AND (created_at::date = $2)", title, createdAt).Scan(&totalPosts)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, err.Error())
		return
	}
	totalPage := int(math.Ceil(float64(totalPosts) / float64(limit)))

	response := Response{
		Posts:     posts,
		Count:     len(posts),
		Limit:     limit,
		Page:      page,
		TotalPage: totalPage,
	}
	respondWithJSON(w, response, http.StatusOK)
}

func getPostsDateRange(w http.ResponseWriter, r *http.Request) {
	// Decode JSON start_date และ end_date
	var requestBody struct {
		StartDate string `json:"start_date"`
		EndDate   string `json:"end_date"`
	}
	err := json.NewDecoder(r.Body).Decode(&requestBody)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, err.Error())
		return
	}

	// ตรวจสอบว่าระบุ start_date และ end_date หรือไม่
	if requestBody.StartDate == "" {
		respondWithError(w, http.StatusBadRequest, "start_date is required")
		return
	}
	if requestBody.EndDate == "" {
		respondWithError(w, http.StatusBadRequest, "end_date is required")
		return
	}

	// start_date และ end_date เป็น time.Time
	startDate, err := time.Parse("2006-01-02", requestBody.StartDate)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Invalid start_date format. Use yyyy-mm-dd.")
		return
	}
	endDate, err := time.Parse("2006-01-02", requestBody.EndDate)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Invalid end_date format. Use yyyy-mm-dd.")
		return
	}

	db, err := connectToDatabase()
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer db.Close()

	queryParams := r.URL.Query()
	pageStr := queryParams.Get("page")
	limitStr := queryParams.Get("limit")

	page, _ := strconv.Atoi(pageStr)
	if page == 0 {
		page = 1
	}
	limit, _ := strconv.Atoi(limitStr)
	if limit == 0 {
		count, err := getDataCount()
		if err != nil {
			respondWithError(w, http.StatusInternalServerError, err.Error())
			return
		} else {
			limit = count
		}
	}
	offset := (page - 1) * limit

	query := "SELECT id, title, content, published, created_at FROM posts WHERE (created_at::date BETWEEN $1 AND $2) LIMIT $3 OFFSET $4"

	rows, err := db.Query(query, startDate.Format("2006-01-02"), endDate.Format("2006-01-02"), limit, offset)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()

	var posts []Post

	for rows.Next() {
		var post Post
		err := rows.Scan(&post.ID, &post.Title, &post.Content, &post.Published, &post.CreatedAt)
		if err != nil {
			respondWithError(w, http.StatusInternalServerError, err.Error())
			return
		}
		posts = append(posts, post)
	}

	var totalPosts int
	err = db.QueryRow("SELECT COUNT(*) FROM posts WHERE (created_at::date BETWEEN $1 AND $2)", startDate.Format("2006-01-02"), endDate.Format("2006-01-02")).Scan(&totalPosts)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, err.Error())
		return
	}

	totalPage := int(math.Ceil(float64(totalPosts) / float64(limit)))

	response := Response{
		Posts:     posts,
		Count:     len(posts),
		Limit:     limit,
		Page:      page,
		TotalPage: totalPage,
	}

	respondWithJSON(w, response, http.StatusOK)
}

// createPostNew สร้าง new post ใน Database
func createPostNew(w http.ResponseWriter, r *http.Request) {
	// Decode ข้อมูล JSON ที่ส่งมาจาก HTTP request body ลงใน struct Post
	var post Post
	err := json.NewDecoder(r.Body).Decode(&post)

	// หากเกิดข้อผิดพลาดในการถอดรหัส JSON ตอบกลับด้วย HTTP status 400 Bad Request
	if err != nil {
		respondWithError(w, http.StatusBadRequest, err.Error())
		return
	}
	// ตรวจสอบว่า title ไม่ว่างเปล่า หากเป็นเช่นนั้น ตอบกลับด้วย HTTP status 400 Bad Request
	if post.Title == "" {
		respondWithError(w, http.StatusBadRequest, "title is required")
		return
	}

	// ตรวจสอบว่า Content เป็นค่าว่างหรือไม่ หากใช่ กำหนดเป็นสตริงว่าง
	if post.Content == "" {
		post.Content = ""
	}

	// กำหนดค่า Published เป็น false เนื่องจากโพสต์ใหม่ยังไม่ได้เผยแพร่
	post.Published = false

	// เชื่อมต่อกับฐานข้อมูล
	db, err := connectToDatabase()
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer db.Close()

	// กำหนด timezone ของไทย
	_, err = db.Exec("SET TIME ZONE 'Asia/Bangkok'")
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// เตรียมคำสั่ง SQL สำหรับการเพิ่ม new post
	stmt, err := db.Prepare("INSERT INTO posts (title, content, published, created_at) VALUES ($1, $2, $3, $4) RETURNING id, created_at")
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer stmt.Close()

	// เวลาปัจจุบัน
	now := time.Now()

	// ใช้คำสั่ง QueryRow เพื่อเพิ่ม new post ลงใน Database และส่งค่า id และ created_at กลับ
	err = stmt.QueryRow(post.Title, post.Content, post.Published, now).Scan(&post.ID, &post.CreatedAt)
	// หากเกิดข้อผิดพลาดในขณะเพิ่มลงในฐานข้อมูล ตอบกลับด้วย HTTP status 500 Internal Server Error และข้อความข้อผิดพลาด
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "error message")
		return
	}

	// ตอบกลับด้วย HTTP status 201 Created
	w.WriteHeader(http.StatusCreated)
	respondWithJSON(w, post, http.StatusCreated)
}

// updatePost อัปเดต post โดยใช้ ID
func updatePost(w http.ResponseWriter, r *http.Request) {
	params := mux.Vars(r)
	postID := params["id"]

	// Decode ข้อมูล JSON ที่ส่งมาจาก HTTP request body ลงใน struct Post
	var updatedPost Post
	err := json.NewDecoder(r.Body).Decode(&updatedPost)
	// หากเกิดข้อผิดพลาดในการถอดรหัส JSON ตอบกลับด้วย HTTP status 400 Bad Request และข้อความข้อผิดพลาด
	if err != nil {
		respondWithError(w, http.StatusBadRequest, err.Error())
		return
	}
	// Connect to the database
	db, err := connectToDatabase()
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer db.Close()

	// คำสั่ง SQL สำหรับการ update post ด้วย ID
	stmt, err := db.Prepare("UPDATE posts SET title = $1, content = $2, published = $3, created_at = $4, updated_at = $5 WHERE id = $6")
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer stmt.Close()

	// ดึงข้อมูล post ที่มีอยู่ใน database
	var existingPost Post
	err = db.QueryRow("SELECT title, content, published, created_at FROM posts WHERE id = $1", postID).Scan(&existingPost.Title, &existingPost.Content, &existingPost.Published, &existingPost.CreatedAt)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, err.Error())
		return
	}
	// อัปเดตฟิลด์ post ด้วยค่าที่มีอยู่หากไม่มีการระบุค่าใหม่
	if updatedPost.Title == "" {
		updatedPost.Title = existingPost.Title
	}
	if updatedPost.Content == "" {
		updatedPost.Content = existingPost.Content
	}
	if updatedPost.CreatedAt.IsZero() {
		updatedPost.CreatedAt = existingPost.CreatedAt
	}
	// กำหนดค่าเวลาปัจจุบันเพื่ออัปเดตฟิลด์ updated_at
	updatedPost.Updated_at = time.Now()

	// อัพเดตโพสต์ในฐานข้อมูล
	_, err = stmt.Exec(updatedPost.Title, updatedPost.Content, updatedPost.Published, updatedPost.CreatedAt, updatedPost.Updated_at, postID)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// รับโพสต์ที่อัปเดตแล้วตอบกลับด้วย JSON
	updatedPost.ID = postID
	respondWithJSON(w, updatedPost, http.StatusOK)
}

// deletePost ลบโพสต์โดยใช้ ID
func deletePost(w http.ResponseWriter, r *http.Request) {
	params := mux.Vars(r)
	postID := params["id"]

	// เชื่อมต่อกับฐานข้อมูล
	db, err := connectToDatabase()
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer db.Close()

	// เตรียมคำสั่ง SQL สำหรับการลบโพสต์โดยใช้ ID
	stmt, err := db.Prepare("DELETE FROM posts WHERE id = $1")
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer stmt.Close()

	// ทำการลบโพสต์ในฐานข้อมูล
	result, err := stmt.Exec(postID)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Failed to delete")
		return
	}

	// ตรวจสอบว่ามีการลบแถวได้หรือไม่
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Failed to delete")
		return
	}
	if rowsAffected == 0 {
		respondWithError(w, http.StatusNotFound, "The post you want to delete was not found.")
		return
	}

	// ตอบกลับด้วย HTTP status 200 OK และข้อความแสดงว่าโพสต์ถูกลบสำเร็จ
	respondWithJSON(w, map[string]string{"message": "Successfully deleted."}, http.StatusOK)
}

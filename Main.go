package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"time"

	_ "github.com/go-sql-driver/mysql"
	"github.com/gorilla/sessions"
	_ "github.com/gorilla/sessions"
)

type DatabaseConfig struct {
	Username string `json:"user"`
	Password string `json:"password"`
	Host     string `json:"host"`
	Port     int    `json:"port"`
	Database string `json:"dbname"`
}

var id bool
var dbc string
var userID int
var store = sessions.NewCookieStore([]byte("logging succes secrect key"))

func logError(err error) {
	if err != nil {
		log.Println(err)
	}
}

func main() {
	logFile, err := os.OpenFile("errors.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		log.Println(err)
	}
	defer logFile.Close()

	log.SetOutput(logFile)

	mux := http.NewServeMux()
	// route naar url
	mux.HandleFunc("/", RootHandler)

	mux.HandleFunc("/Locatie", LocatieHandler)

	mux.HandleFunc("/Login", LoginHandler)

	mux.HandleFunc("/Booking", BookingHandler)
	mux.HandleFunc("/Logout", LogoutHandler)

	// read json file
	configBytes, err := ioutil.ReadFile("Db.json")
	if err != nil {
		logError(err)
	}

	// setting up new struct
	var config DatabaseConfig
	if err := json.Unmarshal(configBytes, &config); err != nil {
		log.Println(err)
	}

	// make connection database
	dbc = fmt.Sprintf("%s:%s@tcp(%s:%d)/%s", config.Username, config.Password, config.Host, config.Port, config.Database)
	db, err := sql.Open("mysql", dbc)
	if err != nil {
		logError(err)
	}
	defer db.Close()

	if err := db.Ping(); err != nil {
		log.Println(err)
	}

	http.ListenAndServe(":8080", mux)
}

type LoginResponse struct {
	Success bool
	UserID  int
}

func checkLogin(w http.ResponseWriter, r *http.Request) LoginResponse {
	username := r.FormValue("username")
	password := r.FormValue("password")

	db, err := sql.Open("mysql", dbc)
	if err != nil {
		log.Println(err)
		return LoginResponse{Success: false, UserID: -1}
	}
	defer db.Close()

	err = db.QueryRow("SELECT id FROM users WHERE username=? AND password=?", username, password).Scan(&userID)
	if err != nil {
		log.Println(err)
		return LoginResponse{Success: false, UserID: -1}
	}
	sessionID := createSession(userID)
	if sessionID == "" {
		log.Println("Error creating session")
		return LoginResponse{Success: false, UserID: -1}
	}
	sessionCookie := &http.Cookie{Name: "session_id", Value: sessionID, Path: "/", Expires: time.Now().Add(time.Duration(24) * time.Hour)}
	http.SetCookie(w, sessionCookie)
	return LoginResponse{Success: true, UserID: userID}
}

func RootHandler(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintln(w, `
		<h1>Fonteyn Vakantieparken</h1>
		<p>Vakantie parken voor een onvergeetelijke vakantie.</p>
		<ul>
			<li><a href="/Locatie">Locatie</a></li>
			<li><a href="/Login">Login</a></li>
		</ul>
	`)
}

func LocatieHandler(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintln(w, `
		<h1>Locatie</h1>
		<p>Wij zijn beschikbaar in meerder landen.</p>
		<ul>
			<li><a href="/">Home</a></li>
			<li><a href="/Login">Login</a></li>
		</ul>
	`)
}
func createSession(userID int) string {
	// Create a new session in the database and return the session ID
	db, err := sql.Open("mysql", dbc)
	if err != nil {
		log.Println(err)
		return ""
	}
	defer db.Close()

	// Create a unique session ID
	sessionID := fmt.Sprintf("%d_%d", userID, time.Now().UnixNano())
	_, err = db.Exec("INSERT INTO sessions (user_id, start_time, end_time) VALUES (?, NOW(), NOW())", userID)
	if err != nil {
		log.Println(err)
		return ""
	}

	return sessionID
}
func LoginHandler(w http.ResponseWriter, r *http.Request) {
	session, err := store.Get(r, "session-name")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return

	}
	session.Values["logged_in"] = false

	session.Save(r, w)
	if r.Method == http.MethodPost {
		loginResponse := checkLogin(w, r)
		if loginResponse.Success {
			http.Redirect(w, r, "/Booking", http.StatusFound)
			return
		}
	}
	fmt.Fprintln(w, `
        <h1>Fonteyn Vakantieparken</h1>
        <p>Vakantie parken voor een onvergeetelijke vakantie.</p>
        <form action="/Login" method="post">
            <label for="username">Username:</label>
            <input type="text" id="username" name="username">
            <br>
            <label for="password">Password:</label>
            <input type="password" id="password" name="password">
            <br><br>
            <input type="submit" value="Submit">
        </form>
    `)
}

func LogoutHandler(w http.ResponseWriter, r *http.Request) {
	sessionCookie, _ := r.Cookie("session_id")
	sessionCookie.MaxAge = -1
	http.SetCookie(w, sessionCookie)
	http.Redirect(w, r, "/", http.StatusFound)
}

func BookingHandler(w http.ResponseWriter, r *http.Request) {
	// Check if the user is logged in
	session, err := store.Get(r, "session-name")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	// Als de gebruiker niet is ingelogd, verwijs de gebruiker door naar de inlogpagina
	if session.Values["logged_in"] != true {
		http.Redirect(w, r, "/login", http.StatusFound)
		return
	}
}

func validateSession(w http.ResponseWriter, r *http.Request, sessionID string) bool {
	db, err := sql.Open("mysql", dbc)
	if err != nil {
		log.Println(err)
		return false
	}
	defer db.Close()

	var userID int
	err = db.QueryRow("SELECT user_id FROM sessions WHERE id = ? AND end_time > NOW()", sessionID).Scan(&userID)
	if err != nil {
		log.Println(err)
		return false
	}
	if userID > 0 {
		session, err1 := store.Get(r, "session-name")

		if err1 != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return false
		}
		session.Values["logged_in"] = true
		session.Save(r, w)
		http.Redirect(w, r, "/booking", http.StatusFound)
		return true
	} else {
		return false
	}
}

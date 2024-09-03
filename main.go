package main

import (
	"context"
	"encoding/json"
	"fmt"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/youtube/v3"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

// OAuth2 konfigürasyonu
const (
	clientID     = "" // Buraya kendi clientID'nizi ekleyin
	clientSecret = "" // Buraya kendi clientSecret'inizi ekleyin
	redirectURL  = "" // Buraya kendi redirectURL'nizi ekleyin
)

// Token dosyasının adı
const tokenFile = "token.json"

// RequestBody represents the expected JSON structure
type RequestBody struct {
	URL string `json:"url"`
}

var oauth2Config = &oauth2.Config{
	ClientID:     clientID,
	ClientSecret: clientSecret,
	RedirectURL:  redirectURL,
	Scopes:       []string{"https://www.googleapis.com/auth/youtube.readonly"},
	Endpoint:     google.Endpoint,
}

func getClient(ctx context.Context) (*http.Client, error) {
	var token *oauth2.Token
	if _, err := os.Stat(tokenFile); err == nil {
		file, err := os.Open(tokenFile)
		if err != nil {
			return nil, fmt.Errorf("error opening token file: %w", err)
		}
		defer file.Close()

		decoder := json.NewDecoder(file)
		token = &oauth2.Token{}
		if err := decoder.Decode(token); err != nil {
			return nil, fmt.Errorf("error decoding token: %w", err)
		}
	} else {
		return nil, fmt.Errorf("token file not found")
	}

	client := oauth2Config.Client(ctx, token)
	return client, nil
}

func authHandler(w http.ResponseWriter, r *http.Request) {
	authURL := oauth2Config.AuthCodeURL("state-token", oauth2.AccessTypeOffline)
	http.Redirect(w, r, authURL, http.StatusFound)
}

func oauth2CallbackHandler(w http.ResponseWriter, r *http.Request) {
	ctx := context.Background()
	code := r.URL.Query().Get("code")
	if code == "" {
		http.Error(w, "Missing authorization code", http.StatusBadRequest)
		return
	}

	token, err := oauth2Config.Exchange(ctx, code)
	if err != nil {
		http.Error(w, fmt.Sprintf("Error exchanging code for token: %v", err), http.StatusInternalServerError)
		return
	}

	file, err := os.Create(tokenFile)
	if err != nil {
		http.Error(w, fmt.Sprintf("Error creating token file: %v", err), http.StatusInternalServerError)
		return
	}
	defer file.Close()

	encoder := json.NewEncoder(file)
	if err := encoder.Encode(token); err != nil {
		http.Error(w, fmt.Sprintf("Error encoding token: %v", err), http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/download", http.StatusFound)
}

func downloadHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Invalid request method", http.StatusMethodNotAllowed)
		return
	}

	var reqBody RequestBody
	if err := json.NewDecoder(r.Body).Decode(&reqBody); err != nil {
		http.Error(w, "Invalid JSON body", http.StatusBadRequest)
		return
	}

	videoURL := reqBody.URL
	if videoURL == "" {
		http.Error(w, "Please provide a video URL", http.StatusBadRequest)
		return
	}

	ctx := context.Background()
	client, err := getClient(ctx)
	if err != nil {
		http.Error(w, fmt.Sprintf("Error getting OAuth2 client: %v", err), http.StatusInternalServerError)
		return
	}

	svc, err := youtube.New(client)
	if err != nil {
		http.Error(w, fmt.Sprintf("Error creating YouTube service: %v", err), http.StatusInternalServerError)
		return
	}

	videoID := extractVideoID(videoURL)
	if videoID == "" {
		http.Error(w, "Invalid video URL", http.StatusBadRequest)
		return
	}

	call := svc.Videos.List([]string{"snippet", "contentDetails", "statistics"}).Id(videoID)
	response, err := call.Do()
	if err != nil {
		http.Error(w, fmt.Sprintf("Error getting video info: %v", err), http.StatusInternalServerError)
		return
	}

	if len(response.Items) == 0 {
		http.Error(w, "Video not found", http.StatusNotFound)
		return
	}

	video := response.Items[0]
	videoTitle := video.Snippet.Title
	fileName := fmt.Sprintf("%s.mp4", strings.ReplaceAll(videoTitle, " ", "_"))
	downloadPath := filepath.Join("downloads", fileName)

	file, err := os.Create(downloadPath)
	if err != nil {
		http.Error(w, fmt.Sprintf("Error creating file: %v", err), http.StatusInternalServerError)
		return
	}
	defer file.Close()

	resp, err := downloadVideo(videoID)
	if err != nil {
		http.Error(w, fmt.Sprintf("Error downloading video: %v", err), http.StatusInternalServerError)
		return
	}
	defer resp.Close()

	_, err = io.Copy(file, resp)
	if err != nil {
		http.Error(w, fmt.Sprintf("Error saving video: %v", err), http.StatusInternalServerError)
		return
	}

	http.ServeFile(w, r, downloadPath)
}

func extractVideoID(videoURL string) string {
	u, err := url.Parse(videoURL)
	if err != nil {
		return ""
	}

	if u.Host == "youtu.be" {
		segments := strings.Split(u.Path, "/")
		if len(segments) > 1 {
			return segments[1]
		}
	}

	if u.Host == "www.youtube.com" || u.Host == "youtube.com" {
		queryParams := u.Query()
		if videoID := queryParams.Get("v"); videoID != "" {
			return videoID
		}
	}

	return ""
}

func downloadVideo(videoID string) (io.ReadCloser, error) {
	// Burada video indirme kodunuzu yazmalısınız
	return nil, fmt.Errorf("video download not implemented")
}

func main() {
	if _, err := os.Stat("downloads"); os.IsNotExist(err) {
		err := os.Mkdir("downloads", 0755)
		if err != nil {
			log.Fatalf("Error creating downloads directory: %v", err)
		}
	}

	http.HandleFunc("/auth", authHandler)
	http.HandleFunc("/oauth2callback", oauth2CallbackHandler)
	http.HandleFunc("/download", downloadHandler)

	fmt.Println("Server is running on http://localhost:8000")
	log.Fatal(http.ListenAndServe(":8000", nil))
}

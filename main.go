package main

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/kkdai/youtube/v2"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// OAuth2 konfigürasyonu
const (
	clientID     = "YOUR_CLIENT_ID"
	clientSecret = "YOUR_CLIENT_SECRET"
	redirectURL  = "http://localhost:8080/oauth2callback"
)

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
	httpClient, err := getClient(ctx)
	if err != nil {
		http.Error(w, fmt.Sprintf("Error getting OAuth2 client: %v", err), http.StatusInternalServerError)
		return
	}

	youtubeClient := youtube.Client{HTTPClient: httpClient}

	video, err := youtubeClient.GetVideo(videoURL)
	if err != nil {
		http.Error(w, fmt.Sprintf("Error getting video info: %v", err), http.StatusInternalServerError)
		return
	}

	formats := video.Formats.WithAudioChannels()
	if len(formats) == 0 {
		http.Error(w, "No suitable formats with audio found", http.StatusInternalServerError)
		return
	}

	format := formats[0]
	for _, f := range formats {
		if f.Height > format.Height {
			format = f
		}
	}

	fileName := fmt.Sprintf("%s.mp4", strings.ReplaceAll(video.Title, " ", "_"))
	downloadPath := filepath.Join("downloads", fileName)

	file, err := os.Create(downloadPath)
	if err != nil {
		http.Error(w, fmt.Sprintf("Error creating file: %v", err), http.StatusInternalServerError)
		return
	}
	defer file.Close()

	resp, _, err := youtubeClient.GetStream(video, &format)
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

	response := map[string]string{
		"message": "Video downloaded successfully",
		"url":     fmt.Sprintf("/downloads/%s", fileName),
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
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

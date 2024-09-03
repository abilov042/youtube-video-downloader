package main

import (
	"encoding/json"
	"fmt"
	"github.com/kkdai/youtube/v2"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// RequestBody represents the expected JSON structure
type RequestBody struct {
	URL string `json:"url"`
}

func downloadHandler(w http.ResponseWriter, r *http.Request) {
	// Only accept POST requests
	if r.Method != http.MethodPost {
		http.Error(w, "Invalid request method", http.StatusMethodNotAllowed)
		return
	}

	// Parse the JSON body
	var reqBody RequestBody
	if err := json.NewDecoder(r.Body).Decode(&reqBody); err != nil {
		http.Error(w, "Invalid JSON body", http.StatusBadRequest)
		return
	}

	// Extract the video URL from the parsed JSON
	videoURL := reqBody.URL
	if videoURL == "" {
		http.Error(w, "Please provide a video URL", http.StatusBadRequest)
		return
	}

	// Create a new YouTube client
	client := youtube.Client{}

	// Get the video information
	video, err := client.GetVideo(videoURL)
	if err != nil {
		http.Error(w, fmt.Sprintf("Error getting video info: %v", err), http.StatusInternalServerError)
		return
	}

	// Select the format with both video and audio
	formats := video.Formats.WithAudioChannels()
	if len(formats) == 0 {
		http.Error(w, "No suitable formats with audio found", http.StatusInternalServerError)
		return
	}

	// Select the best available format with both video and audio
	format := formats[0]
	for _, f := range formats {
		if f.Height > format.Height {
			format = f
		}
	}

	// Define the download path
	fileName := fmt.Sprintf("%s.mp4", strings.ReplaceAll(video.Title, " ", "_"))
	downloadPath := filepath.Join("downloads", fileName)

	// Open a file to save the video
	file, err := os.Create(downloadPath)
	if err != nil {
		http.Error(w, fmt.Sprintf("Error creating file: %v", err), http.StatusInternalServerError)
		return
	}
	defer file.Close()

	// Download the video with audio
	resp, _, err := client.GetStream(video, &format)
	if err != nil {
		http.Error(w, fmt.Sprintf("Error downloading video: %v", err), http.StatusInternalServerError)
		return
	}
	defer resp.Close()

	// Save the video to the file
	_, err = io.Copy(file, resp)
	if err != nil {
		http.Error(w, fmt.Sprintf("Error saving video: %v", err), http.StatusInternalServerError)
		return
	}

	// Respond with the downloaded video file
	w.Header().Set("Content-Disposition", "attachment; filename="+fileName)
	w.Header().Set("Content-Type", "video/mp4")
	http.ServeFile(w, r, downloadPath)
}

func main() {
	// Create the downloads directory if it doesn't exist
	if _, err := os.Stat("downloads"); os.IsNotExist(err) {
		err := os.Mkdir("downloads", 0755)
		if err != nil {
			log.Fatalf("Error creating downloads directory: %v", err)
		}
	}

	http.HandleFunc("/download", downloadHandler)

	fmt.Println("Server is running on http://localhost:8000")
	log.Fatal(http.ListenAndServe(":8000", nil))
}

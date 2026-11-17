package main

import (
	"log"
	"net/http"
	"os"
)

func streamHandler(videoPath string) http.HandlerFunc{
	return func(w http.ResponseWriter, r *http.Request) {
		file,err := os.Open(videoPath)

		if err != nil {
			http.Error(w,"Video not found",http.StatusNotFound)
			return
		}

		defer file.Close()

		stat,err := file.Stat()
		if err != nil {
			http.Error(w,"Could not read video metadata",http.StatusInternalServerError)
			return 
		}

		w.Header().Set("Content-Type","video/mp4")
		http.ServeContent(w,r,stat.Name(),stat.ModTime(),file)
	}
}

func main(){
	// http.HandleFunc("/stream",streamHandler("song.mp3"))

	http.HandleFunc("/stream",streamHandler("video.mp4"))

	log.Println("Server running on http://localhost:8080")
	if err := http.ListenAndServe(":8080",nil);err != nil{
		log.Fatal(err)
	}
}
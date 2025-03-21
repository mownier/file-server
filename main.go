// main.go
package main

import (
	"bufio"
	"embed"
	"encoding/json"
	"file-server/models"
	"fmt"
	"html/template"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

//go:embed templates/* static/*
var content embed.FS

func main() {
	reader := bufio.NewReader(os.Stdin)

	fmt.Print("Enter the port to listen on (or press Enter for 8080): ")
	port, _ := reader.ReadString('\n')
	port = strings.TrimSpace(port)

	if port == "" {
		port = "8080"
	}

	fmt.Print("Enter directories to serve (comma-separated, or press Enter for current directory): ")
	dirsInput, _ := reader.ReadString('\n')
	dirsInput = strings.TrimSpace(dirsInput)

	directories := []string{"."}
	if dirsInput != "" {
		directories = strings.Split(dirsInput, ",")
		for i := range directories {
			directories[i] = strings.TrimSpace(directories[i])
		}
	}

	fmt.Print("Select option (videos/all or press enter for all): ")
	option, _ := reader.ReadString('\n')
	option = strings.TrimSpace(strings.ToLower(option))

	validOptions := []string{"videos", "all", ""}

	if !contains(validOptions, option) {
		log.Fatalf("Option %s NOT valid", option)
	}

	addrs, err := net.InterfaceAddrs()
	if err != nil {
		log.Fatalf("Error getting network interfaces: %v", err)
	}

	if option == "videos" {
		videoExtensions := []string{".mp4", ".avi", ".mkv", ".mov", ".webm"}
		fmt.Print("Enter additional file extensions to serve (comma-separated, or press Enter for default): ")
		fileExtensionsInput, _ := reader.ReadString('\n')
		fileExtensionsInput = strings.TrimSpace(fileExtensionsInput)
		if fileExtensionsInput != "" {
			fileExtensions := strings.Split(fileExtensionsInput, ",")
			for i := range fileExtensions {
				fileExtension := strings.TrimSpace(fileExtensions[i])
				videoExtensions = append(videoExtensions, fileExtension)
			}
		}
		fmt.Printf("%v\n", videoExtensions)
		videos(directories, addrs, port, videoExtensions)
	}

	if option == "" || option == "all" {
		all(directories, addrs, port)
	}

	for _, addr := range addrs {
		if ipnet, ok := addr.(*net.IPNet); ok && !ipnet.IP.IsLoopback() {
			if ipnet.IP.To4() != nil {
				localIP := ipnet.IP.String()
				fmt.Printf("Server listening on: http://%s:%s\n", localIP, port)
			}
		}
	}

	http.Handle("/static/", http.FileServer(http.FS(content)))

	addr := ":" + port
	err = http.ListenAndServe(addr, nil)
	if err != nil {
		log.Fatalf("Error starting server: %v", err)
	}
}

func all(directories []string, addrs []net.Addr, port string) {
	folderURLs := make(map[string]models.Folder)

	for i, dir := range directories {
		absDir, err := filepath.Abs(dir)
		if err != nil {
			log.Printf("Error getting absolute path for %s: %v", dir, err)
			continue
		}
		prefix := fmt.Sprintf("/dir%d/", i+1)
		folderURLs[prefix] = models.Folder{Name: prefix, Path: absDir}
	}

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			tmpl := template.Must(template.ParseFS(content, "templates/welcome.html"))
			err := tmpl.Execute(w, folderURLs)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
			}
			return
		}
		for _, folder := range folderURLs {
			if r.URL.Path == folder.Name {
				http.ServeFile(w, r, folder.Path)
				return
			}
			if r.URL.Path == strings.TrimSuffix(folder.Name, "/") {
				http.Redirect(w, r, folder.Name, http.StatusMovedPermanently)
				return
			}
		}
	})

	localIP := ""

	for _, addr := range addrs {
		if ipnet, ok := addr.(*net.IPNet); ok && !ipnet.IP.IsLoopback() {
			if ipnet.IP.To4() != nil {
				localIP = ipnet.IP.String()
			}
		}
	}

	for _, folder := range folderURLs {
		fmt.Printf("URL: http://%s:%s%s -> Folder: %s\n", localIP, port, folder.Name, folder.Path)
	}
}

func videos(directories []string, addrs []net.Addr, port string, extensions []string) {
	videoURLs := make(map[string][]models.Video) // Map directory URL to list of videos

	counter := 1
	for _, dir := range directories {
		absDir, err := filepath.Abs(dir)
		if err != nil {
			log.Printf("Error getting absolute path for %s: %v", dir, err)
			continue
		}

		prefix := fmt.Sprintf("/dir%d/", counter)

		videoCounter := 1 // Counter for videos within each directory
		err = filepath.Walk(absDir, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if !info.IsDir() && isVideo(info.Name(), extensions) {
				aliasPath := fmt.Sprintf("video%d", videoCounter) // Alias path (no leading slash)
				videoURLs[prefix] = append(videoURLs[prefix], models.Video{Name: info.Name(), Path: prefix + aliasPath, FilePath: path})
				videoCounter++
			}
			return nil
		})
		if err != nil {
			log.Printf("Error walking directory %s: %v", absDir, err)
		}
		counter++
	}

	http.HandleFunc("/videourls", func(w http.ResponseWriter, r *http.Request) {
		folders := []models.PublicVideoFolder{}
		for dir, videos := range videoURLs {
			publicVideos := []models.PublicVideo{}
			for _, video := range videos {
				publicVideos = append(publicVideos, models.PublicVideo{Name: video.Name, Path: video.Path})
			}
			folders = append(folders, models.PublicVideoFolder{Name: dir, Videos: publicVideos})
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(models.VideoURLListResponse{Folders: folders})
	})

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			tmpl := template.Must(template.ParseFS(content, "templates/welcome.html"))
			err := tmpl.Execute(w, videoURLs)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
			}
			return
		}

		for dirURL, urls := range videoURLs {
			if r.URL.Path == dirURL {
				tmpl := template.Must(template.ParseFS(content, "templates/videos.html"))
				err := tmpl.Execute(w, urls)
				if err != nil {
					http.Error(w, err.Error(), http.StatusInternalServerError)
				}
				return
			}
			if r.URL.Path == strings.TrimSuffix(dirURL, "/") {
				http.Redirect(w, r, dirURL, http.StatusMovedPermanently)
				return
			}
		}

		for _, videos := range videoURLs {
			for _, video := range videos {
				if r.URL.Path == video.Path {
					http.ServeFile(w, r, video.FilePath)
					return
				}
			}
		}
		http.NotFound(w, r)
	})

	localIP := ""

	for _, addr := range addrs {
		if ipnet, ok := addr.(*net.IPNet); ok && !ipnet.IP.IsLoopback() {
			if ipnet.IP.To4() != nil {
				localIP = ipnet.IP.String()
			}
		}
	}

	for _, videos := range videoURLs {
		for _, video := range videos {
			fmt.Printf("URL: http://%s:%s%s -> File: %s\n", localIP, port, video.Path, video.FilePath)
		}
	}
}

func isVideo(filename string, extensions []string) bool {
	for _, ext := range extensions {
		if strings.HasSuffix(strings.ToLower(filename), ext) {
			return true
		}
	}
	return false
}

func contains(slice []string, target string) bool {
	for _, str := range slice {
		if str == target {
			return true
		}
	}
	return false
}
